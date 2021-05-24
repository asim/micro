package broker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/micro/micro/v3/service/broker"
	"github.com/micro/micro/v3/service/events"
	"github.com/micro/micro/v3/service/logger"
	"github.com/micro/micro/v3/service/registry/mdns"
	"github.com/pkg/errors"
)

var (
	errHandler = fmt.Errorf("error from handler")
)

type redisBroker struct {
	redisClient *redis.Client
	opts        broker.Options
	ropts       Options
}

type subscriber struct {
	opts   broker.SubscribeOptions
	broker *redisBroker
	topic  string
	queue  string
}

func (s *subscriber) Options() broker.SubscribeOptions {
	return s.opts
}

func (s *subscriber) Topic() string {
	return s.topic
}

func (s subscriber) Unsubscribe() error {
	return s.broker.redisClient.XGroupDestroy(context.Background(), fmt.Sprintf("broker-%s", s.topic), s.queue).Err()
}

func (r *redisBroker) Init(opts ...broker.Option) error {
	r.setOption(opts...)
	return nil
}

func (r *redisBroker) Options() broker.Options {
	return r.opts
}

func (r *redisBroker) Address() string {
	return r.redisClient.Options().Addr
}

func (r *redisBroker) Connect() error {
	return nil
}

func (r *redisBroker) Disconnect() error {
	return r.redisClient.Close()
}

func (r *redisBroker) Publish(topic string, m *broker.Message, opts ...broker.PublishOption) error {
	// validate the topic
	if len(topic) == 0 {
		return errors.New("missing topic")
	}
	topic = fmt.Sprintf("broker-%s", topic)

	payload, err := r.opts.Codec.Marshal(m)
	if err != nil {
		return err
	}

	return r.redisClient.XAdd(context.Background(), &redis.XAddArgs{
		Stream: topic,
		Values: []string{"event", string(payload)},
	}).Err()

}

func (r *redisBroker) Subscribe(topic string, h broker.Handler, opts ...broker.SubscribeOption) (broker.Subscriber, error) {
	if len(topic) == 0 {
		return nil, events.ErrMissingTopic
	}

	opt := broker.SubscribeOptions{
		Context: context.Background(),
	}

	for _, o := range opts {
		o(&opt)
	}

	group := opt.Queue
	if len(group) == 0 {
		group = uuid.New().String()
	}
	ret := subscriber{
		opts:   opt,
		broker: r,
		topic:  topic,
		queue:  group,
	}

	err := r.consumeWithGroup(fmt.Sprintf("broker-%s", topic), group, h, opt.ErrorHandler)
	if err != nil {
		return nil, err
	}
	return &ret, nil

}

func (r *redisBroker) String() string {
	return "redis"
}

func (r *redisBroker) consumeWithGroup(topic, group string, h broker.Handler, eh broker.ErrorHandler) error {
	lastRead := "$"
	if err := r.redisClient.XGroupCreateMkStream(context.Background(), topic, group, lastRead).Err(); err != nil {
		if !strings.HasPrefix(err.Error(), "BUSYGROUP") {
			return err
		}
	}
	consumerName := uuid.New().String()
	go func() {
		defer func() {
			// try to clean up the consumer
			r.redisClient.XGroupDelConsumer(context.Background(), topic, group, consumerName)
		}()
		// only stop processing if we get an error while from the handler, in all other cases continue
		for {
			// sweep up any old pending messages
			pend, err := r.redisClient.XPendingExt(context.Background(), &redis.XPendingExtArgs{
				Stream: topic,
				Group:  group,
				Start:  "-",
				End:    "+",
				Count:  50,
			}).Result()
			if err != nil && err != redis.Nil {
				logger.Errorf("Error finding pending messages %s", err)
				return
			}
			pendingIDs := make([]string, len(pend))
			for i, p := range pend {
				pendingIDs[i] = p.ID
			}
			if len(pendingIDs) > 0 {
				msgs, err := r.redisClient.XClaim(context.Background(), &redis.XClaimArgs{
					Stream:   topic,
					Group:    group,
					Consumer: consumerName,
					MinIdle:  60 * time.Second,
				}).Result()
				if err != nil && err != redis.Nil {
					logger.Errorf("Error claiming message %s", err)
					continue
				}
				if err := r.processMessages(msgs, topic, group, h, eh); err == errHandler {
					logger.Errorf("Error processing message %s", err)
					return
				}
			}
			for {
				res := r.redisClient.XReadGroup(context.Background(), &redis.XReadGroupArgs{
					Group:    group,
					Consumer: consumerName,
					Streams:  []string{topic, ">"},
					Block:    0,
				})
				sl, err := res.Result()
				if err != nil {
					logger.Errorf("Error reading from stream %s", err)
					continue
				}
				if sl == nil || len(sl) == 0 {
					logger.Errorf("No data received from stream")
					continue
				}
				if err := r.processMessages(sl[0].Messages, topic, group, h, eh); err == errHandler {
					return
				}
			}
		}

	}()
	return nil
}

func (r *redisBroker) processMessages(msgs []redis.XMessage, topic, group string, h broker.Handler, eh broker.ErrorHandler) error {
	for _, v := range msgs {
		evBytes := v.Values["event"]
		bStr, ok := evBytes.(string)
		if !ok {
			logger.Warnf("Failed to convert to bytes, discarding %s", v.ID)
			r.redisClient.XAck(context.Background(), topic, group, v.ID)
			continue
		}
		var msg broker.Message
		if err := r.opts.Codec.Unmarshal([]byte(bStr), &msg); err != nil {
			logger.Warnf("Failed to unmarshal event, discarding %s %s", err, v.ID)
			r.redisClient.XAck(context.Background(), topic, group, v.ID)
			continue
		}
		if err := h(&msg); err != nil {
			if eh != nil {
				eh(&msg, err)
			}
			return errHandler
		}

		// TODO check for error
		r.redisClient.XAck(context.Background(), topic, group, v.ID)
	}
	return nil
}

func NewBroker(opts ...broker.Option) broker.Broker {
	boptions := broker.Options{
		// Default codec
		Codec:    Marshaler{},
		Context:  context.Background(),
		Registry: mdns.NewRegistry(),
	}

	rs := &redisBroker{
		opts: boptions,
	}
	rs.setOption(opts...)
	return rs
}

func (r *redisBroker) setOption(opts ...broker.Option) {
	for _, o := range opts {
		o(&r.opts)
	}
	// if no specific redis options passed then parse the broker address
	if ropts, ok := r.opts.Context.Value(optionsKey{}).(Options); ok {
		r.ropts = ropts
	} else {
		url, err := redis.ParseURL(r.opts.Addrs[0])
		if err != nil {
			panic(err)
		}
		r.ropts = Options{
			Address:   url.Addr,
			User:      url.Username,
			Password:  url.Password,
			TLSConfig: url.TLSConfig,
		}
	}
	rc := redis.NewClient(&redis.Options{
		Addr:        r.ropts.Address,
		Username:    r.ropts.User,
		Password:    r.ropts.Password,
		TLSConfig:   r.ropts.TLSConfig,
		ReadTimeout: 5 * time.Second,
	})
	r.redisClient = rc
}
