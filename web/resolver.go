package web

import (
	"errors"
	"net"
	"net/http"
	"strings"

	res "github.com/micro/go-micro/v2/api/resolver"
	"github.com/micro/go-micro/v2/client/selector"
	"golang.org/x/net/publicsuffix"
)

type resolver struct {
	// Type of resolver e.g path, domain
	Type string
	// our internal namespace e.g go.micro.web
	Namespace string
	// selector to find services
	Selector selector.Selector
}

func reverse(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func (r *resolver) String() string {
	return "web/resolver"
}

// Info checks whether this is a web request.
// It returns host, namespace and whether its internal
func (r *resolver) Info(req *http.Request) (string, string, bool) {
	// set to host
	host := req.URL.Hostname()

	// set as req.Host if blank
	if len(host) == 0 {
		host = req.Host
	}

	// split out ip
	h, _, err := net.SplitHostPort(host)
	if err == nil {
		host = h
	}

	// now try parse out ip
	ip := net.ParseIP(host)
	dom := host

	// replace our suffix if it exists
	if strings.HasSuffix(host, "micro.mu") {
		dom = strings.Replace(host, "micro.mu", "micro.go", 1)
	}

	// split and reverse the host
	parts := strings.Split(dom, ".")
	reverse(parts)
	namespace := strings.Join(parts, ".")
	// check if its localhost or an ip
	localhost := (ip != nil || host == "localhost")

	// isWeb sets if its a web.micro.mu request
	var isWeb bool

	// go.micro.web => go.micro.web
	// use path based resolution if hostname matches
	// namespace or IP is not nil
	if r.Type == "path" || namespace == r.Namespace || localhost || len(host) == 0 || host == Host {
		isWeb = true
	}

	// is a subdomain request
	return host, namespace, isWeb
}

// Resolve replaces the values of Host, Path, Scheme to calla backend service
// It accounts for subdomains for service names based on namespace
func (r *resolver) Resolve(req *http.Request) (*res.Endpoint, error) {
	// get host, namespace and if its an internal request
	host, namespace, webReq := r.Info(req)

	// use path based resolution if its web dashboard related
	if webReq {
		parts := strings.Split(req.URL.Path, "/")
		if len(parts) < 2 {
			return nil, errors.New("unknown service")
		}

		if !re.MatchString(parts[1]) {
			return nil, res.ErrInvalidPath
		}

		next, err := r.Selector.Select(r.Namespace + "." + parts[1])
		if err == selector.ErrNotFound {
			return nil, res.ErrNotFound
		} else if err != nil {
			return nil, err
		}

		// TODO: better retry strategy
		s, err := next()
		if err != nil {
			return nil, err
		}

		// we're done
		return &res.Endpoint{
			Name:   parts[1],
			Method: req.Method,
			Host:   s.Address,
			Path:   "/" + strings.Join(parts[2:], "/"),
		}, nil
	}

	// create an alias
	var alias string

	// check if suffix is web.micro.go in which case its subdomain + namespace
	if strings.HasPrefix(namespace, r.Namespace) {
		subdomain := strings.TrimPrefix(namespace, r.Namespace+".")
		// split it
		parts := strings.Split(subdomain, ".")
		// reverse it
		reverse(parts)
		// turn it into an alias
		alias = strings.Join(parts, ".")
	} else {
		// namespace does not match so we'll try check subdomain
		domain, err := publicsuffix.EffectiveTLDPlusOne(host)
		if err != nil {
			// fallback
			return nil, err
		}

		// get the subdomain
		subdomain := strings.TrimSuffix(host, "."+domain)
		// split it
		parts := strings.Split(subdomain, ".")
		// reverse it
		reverse(parts)
		// turn it into an alias
		alias = strings.Join(parts, ".")
	}

	// only one part
	if len(alias) > 0 {
		// set name to lookup
		name := r.Namespace + "." + alias

		// get namespace + subdomain
		next, err := r.Selector.Select(name)
		if err == selector.ErrNotFound {
			return nil, res.ErrNotFound
		} else if err != nil {
			return nil, err
		}

		// TODO: better retry strategy
		s, err := next()
		if err != nil {
			return nil, err
		}

		// we're done
		return &res.Endpoint{
			Name:   alias,
			Method: req.Method,
			Host:   s.Address,
			Path:   req.URL.Path,
		}, nil
	}

	// ugh
	return nil, errors.New("unknown host")
}
