// Package runtime is the micro runtime
package runtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	golog "github.com/micro/go-micro/v3/logger"
	"github.com/micro/go-micro/v3/runtime/local/source/git"
	"github.com/micro/micro/v3/client/cli/namespace"
	"github.com/micro/micro/v3/client/cli/util"
	"github.com/micro/micro/v3/internal/config"
	"github.com/micro/micro/v3/service/logger"
	"github.com/micro/micro/v3/service/runtime"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc/status"
)

const (
	// RunUsage message for the run command
	RunUsage = "Run a service: micro run [source]"
	// KillUsage message for the kill command
	KillUsage = "Kill a service: micro kill [source]"
	// UpdateUsage message for the update command
	UpdateUsage = "Update a service: micro update [source]"
	// GetUsage message for micro get command
	GetUsage = "Get the status of services"
	// ServicesUsage message for micro services command
	ServicesUsage = "micro services"
	// CannotWatch message for the run command
	CannotWatch = "Cannot watch filesystem on this runtime"
)

var (
	// DefaultRetries which should be attempted when starting a service
	DefaultRetries = 3
	// DefaultImage which should be run
	// DefaultImage = "micro/cells:micro"
	DefaultImage = "localhost:5000/cells:micro"
	// Git orgs we currently support for credentials
	GitOrgs = []string{"github", "bitbucket", "gitlab"}
)

const (
	credentialsKey = "GIT_CREDENTIALS"
)

// timeAgo returns the time passed
func timeAgo(v string) string {
	if len(v) == 0 {
		return "unknown"
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return v
	}

	return fmt.Sprintf("%v ago", fmtDuration(time.Since(t)))
}

func fmtDuration(d time.Duration) string {
	// round to secs
	d = d.Round(time.Second)

	var resStr string
	days := d / (time.Hour * 24)
	if days > 0 {
		d -= days * time.Hour * 24
		resStr = fmt.Sprintf("%dd", days)
	}
	h := d / time.Hour
	if len(resStr) > 0 || h > 0 {
		d -= h * time.Hour
		resStr = fmt.Sprintf("%s%dh", resStr, h)
	}
	m := d / time.Minute
	if len(resStr) > 0 || m > 0 {
		d -= m * time.Minute
		resStr = fmt.Sprintf("%s%dm", resStr, m)
	}
	s := d / time.Second
	resStr = fmt.Sprintf("%s%ds", resStr, s)
	return resStr
}

// exists returns whether the given file or directory exists
func dirExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func sourceExists(source *git.Source) error {
	ref := source.Ref
	if ref == "" || ref == "latest" {
		ref = "master"
	}

	sourceExistsAt := func(url string, source *git.Source) error {
		req, _ := http.NewRequest("GET", url, nil)

		// add the git credentials if set
		if creds, ok := getGitCredentials(source.Repo); ok {
			req.Header.Set("Authorization", "token "+creds)
		}

		client := new(http.Client)
		resp, err := client.Do(req)

		// @todo gracefully degrade?
		if err != nil {
			return err
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return fmt.Errorf("service at %v@%v not found", source.RuntimeSource(), ref)
		}
		return nil
	}

	if strings.Contains(source.Repo, "github") {
		// Github specific existence checs
		repo := strings.ReplaceAll(source.Repo, "github.com/", "")
		url := fmt.Sprintf("https://api.github.com/repos/%v/contents/%v?ref=%v", repo, source.Folder, ref)
		return sourceExistsAt(url, source)
	} else if strings.Contains(source.Repo, "gitlab") {
		// Gitlab specific existence checks

		// @todo better check for gitlab
		url := fmt.Sprintf("https://%v", source.Repo)
		return sourceExistsAt(url, source)
	}
	return nil
}

func appendSourceBase(ctx *cli.Context, workDir, source string) string {
	isLocal, _ := git.IsLocal(workDir, source)
	// @todo add list of supported hosts here or do this check better
	if !isLocal && !strings.Contains(source, ".com") && !strings.Contains(source, ".org") && !strings.Contains(source, ".net") {
		baseURL, _ := config.Get(config.Path("git", util.GetEnv(ctx).Name, "baseurl"))
		if len(baseURL) == 0 {
			baseURL, _ = config.Get(config.Path("git", "baseurl"))
		}
		if len(baseURL) == 0 {
			return path.Join("github.com/micro/services", source)
		}
		return path.Join(baseURL, source)
	}
	return source
}

func runService(ctx *cli.Context) error {
	// we need some args to run
	if ctx.Args().Len() == 0 {
		fmt.Println(RunUsage)
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	// determine the type of source input, i.e. is it a local folder or a remote git repo
	source, err := git.ParseSourceLocal(wd, appendSourceBase(ctx, wd, ctx.Args().Get(0)))
	if err != nil {
		return err
	}

	// if the source isn't local, ensure it exists
	if !source.Local {
		if err := sourceExists(source); err != nil {
			return err
		}
	}

	// parse the various flags
	typ := ctx.String("type")
	command := strings.TrimSpace(ctx.String("command"))
	args := strings.TrimSpace(ctx.String("args"))
	retries := DefaultRetries
	image := DefaultImage
	if ctx.IsSet("retries") {
		retries = ctx.Int("retries")
	}
	if ctx.IsSet("image") {
		image = ctx.String("image")
	}

	var runtimeSource string
	if source.Local {
		// for local source, upload it to the server and use the resulting source ID
		runtimeSource, err = upload(ctx, source)
		if err != nil {
			return err
		}
	} else {
		// if we're running a remote git repository, pass this as the source
		runtimeSource = source.RuntimeSource()
	}

	// for local source, the srv.Source attribute will be remapped to the id of the source upload.
	// however this won't make sense from a user experience perspective, so we'll pass the argument
	// they used in metadata, e.g. ./helloworld
	metadata := map[string]string{
		"source": source.RuntimeSource(),
	}

	// specify the options
	opts := []runtime.CreateOption{
		runtime.WithOutput(os.Stdout),
		runtime.WithRetries(retries),
		runtime.CreateImage(image),
		runtime.CreateType(typ),
	}
	if len(command) > 0 {
		opts = append(opts, runtime.WithCommand(strings.Split(command, " ")...))
	}
	if len(args) > 0 {
		opts = append(opts, runtime.WithArgs(strings.Split(args, " ")...))
	}

	// when the repo root doesn't match the full path (e.g. in cases where a mono-repo is being
	// used), find the relative path and pass this in the metadata as entrypoint.
	if source.Local && source.LocalRepoRoot != source.FullPath {
		ep, _ := filepath.Rel(source.LocalRepoRoot, source.FullPath)
		opts = append(opts, runtime.CreateEntrypoint(ep))
	}

	// add environment variable passed in via cli
	var environment []string
	for _, evar := range ctx.StringSlice("env_vars") {
		for _, e := range strings.Split(evar, ",") {
			if len(e) > 0 {
				environment = append(environment, strings.TrimSpace(e))
			}
		}
	}
	if len(environment) > 0 {
		opts = append(opts, runtime.WithEnv(environment))
	}

	// determine the namespace
	ns, err := namespace.Get(util.GetEnv(ctx).Name)
	if err != nil {
		return err
	}
	opts = append(opts, runtime.CreateNamespace(ns))
	gitCreds, ok := getGitCredentials(source.Repo)
	if ok {
		opts = append(opts, runtime.WithSecret(credentialsKey, gitCreds))
	}

	// run the service
	return runtime.Create(&runtime.Service{
		Name:     source.RuntimeName(),
		Source:   runtimeSource,
		Version:  source.Ref,
		Metadata: metadata,
	}, opts...)
}

func getGitCredentials(repo string) (string, bool) {
	repo = strings.Split(repo, "/")[0]

	for _, org := range GitOrgs {
		if !strings.Contains(repo, org) {
			continue
		}

		// check the creds for the org
		creds, err := config.Get(config.Path("git", "credentials", org))
		if err == nil && len(creds) > 0 {
			return creds, true
		}
	}

	return "", false
}

func killService(ctx *cli.Context) error {
	// we need some args to run
	if ctx.Args().Len() == 0 {
		fmt.Println(KillUsage)
		return nil
	}

	name := ctx.Args().Get(0)
	ref := ""
	if parts := strings.Split(name, "@"); len(parts) > 1 {
		name = parts[0]
		ref = parts[1]
	}
	if ref == "" {
		ref = "latest"
	}
	service := &runtime.Service{
		Name:    name,
		Version: ref,
	}

	// determine the namespace
	ns, err := namespace.Get(util.GetEnv(ctx).Name)
	if err != nil {
		return err
	}

	if err := runtime.Delete(service, runtime.DeleteNamespace(ns)); err != nil {
		return err
	}

	return nil
}

func updateService(ctx *cli.Context) error {
	// we need some args to run
	if ctx.Args().Len() == 0 {
		fmt.Println(RunUsage)
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	source, err := git.ParseSourceLocal(wd, appendSourceBase(ctx, wd, ctx.Args().Get(0)))
	if err != nil {
		return err
	}

	// if the source isn't local, ensure it exists
	if !source.Local {
		if err := sourceExists(source); err != nil {
			return err
		}
	}

	var runtimeSource string
	if source.Local {
		// for local source, upload it to the server and use the resulting source ID
		runtimeSource, err = upload(ctx, source)
		if err != nil {
			return err
		}
	} else {
		// if we're running a remote git repository, pass this as the source
		runtimeSource = source.RuntimeSource()
	}

	// when the repo root doesn't match the full path (e.g. in cases where a mono-repo is being
	// used), find the relative path and pass this in the metadata as entrypoint
	var opts []runtime.UpdateOption
	if source.Local && source.LocalRepoRoot != source.FullPath {
		ep, _ := filepath.Rel(source.LocalRepoRoot, source.FullPath)
		opts = append(opts, runtime.UpdateEntrypoint(ep))
	}

	// determine the namespace
	ns, err := namespace.Get(util.GetEnv(ctx).Name)
	if err != nil {
		return err
	}
	opts = append(opts, runtime.UpdateNamespace(ns))

	// pass git credentials incase a private repo needs to be pulled
	gitCreds, ok := getGitCredentials(source.Repo)
	if ok {
		opts = append(opts, runtime.UpdateSecret(credentialsKey, gitCreds))
	}

	return runtime.Update(&runtime.Service{
		Name:    source.RuntimeName(),
		Source:  runtimeSource,
		Version: source.Ref,
	}, opts...)
}

func getService(ctx *cli.Context) error {
	name := ""
	version := "latest"
	typ := ctx.String("type")

	if ctx.Args().Len() > 0 {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		source, err := git.ParseSourceLocal(wd, ctx.Args().Get(0))
		if err != nil {
			return err
		}
		name = source.RuntimeName()
	}
	// set version as second arg
	if ctx.Args().Len() > 1 {
		version = ctx.Args().Get(1)
	}

	// should we list sevices
	var list bool

	// zero args so list all
	if ctx.Args().Len() == 0 {
		list = true
	}

	var services []*runtime.Service
	var readOpts []runtime.ReadOption

	// return a list of services
	switch list {
	case true:
		// return specific type listing
		if len(typ) > 0 {
			readOpts = append(readOpts, runtime.ReadType(typ))
		}
	// return one service
	default:
		// check if service name was passed in
		if len(name) == 0 {
			fmt.Println(GetUsage)
			return nil
		}

		// get service with name and version
		readOpts = []runtime.ReadOption{
			runtime.ReadService(name),
			runtime.ReadVersion(version),
		}

		// return the runtime services
		if len(typ) > 0 {
			readOpts = append(readOpts, runtime.ReadType(typ))
		}
	}

	// determine the namespace
	ns, err := namespace.Get(util.GetEnv(ctx).Name)
	if err != nil {
		return err
	}
	readOpts = append(readOpts, runtime.ReadNamespace(ns))

	// read the service
	services, err = runtime.Read(readOpts...)
	if err != nil {
		return err
	}

	// make sure we return UNKNOWN when empty string is supplied
	parse := func(m string) string {
		if len(m) == 0 {
			return "n/a"
		}
		return m
	}

	// don't do anything if there's no services
	if len(services) == 0 {
		return nil
	}

	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })

	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)
	fmt.Fprintln(writer, "NAME\tVERSION\tSOURCE\tSTATUS\tBUILD\tUPDATED\tMETADATA")
	for _, service := range services {
		// cut the commit down to first 7 characters
		build := parse(service.Metadata["build"])
		if len(build) > 7 {
			build = build[:7]
		}

		// if there is an error, display this in metadata (there is no error field)
		metadata := fmt.Sprintf("owner=%s, group=%s", parse(service.Metadata["owner"]), parse(service.Metadata["group"]))
		if service.Status == runtime.Error {
			metadata = fmt.Sprintf("%v, error=%v", metadata, parse(service.Metadata["error"]))
		}

		// parse when the service was started
		updated := parse(timeAgo(service.Metadata["started"]))

		// sometimes the services's source can be remapped to the build id etc, however the original
		// argument passed to micro run is always kept in the source attribute of service metadata
		if src, ok := service.Metadata["source"]; ok {
			service.Source = src
		}

		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			service.Name,
			parse(service.Version),
			parse(service.Source),
			humanizeStatus(service.Status),
			build,
			updated,
			metadata)
	}
	writer.Flush()
	return nil
}

const (
	// logUsage message for logs command
	logUsage = "Required usage: micro log example"
)

func getLogs(ctx *cli.Context) error {
	logger.DefaultLogger.Init(golog.WithFields(map[string]interface{}{"service": "runtime"}))
	if ctx.Args().Len() == 0 {
		fmt.Println("Service name is required")
		return nil
	}

	name := ctx.Args().Get(0)

	// must specify service name
	if len(name) == 0 {
		fmt.Println(logUsage)
		return nil
	}

	// get the args
	options := []runtime.LogsOption{}

	count := ctx.Int("lines")
	if count > 0 {
		options = append(options, runtime.LogsCount(int64(count)))
	} else {
		options = append(options, runtime.LogsCount(int64(15)))
	}

	follow := ctx.Bool("follow")

	if follow {
		options = append(options, runtime.LogsStream(follow))
	}

	// @todo reintroduce since
	//since := ctx.String("since")
	//var readSince time.Time
	//d, err := time.ParseDuration(since)
	//if err == nil {
	//	readSince = time.Now().Add(-d)
	//}

	// determine the namespace
	ns, err := namespace.Get(util.GetEnv(ctx).Name)
	if err != nil {
		return err
	}
	options = append(options, runtime.LogsNamespace(ns))

	logs, err := runtime.Log(&runtime.Service{Name: name}, options...)

	if err != nil {
		return err
	}

	output := ctx.String("output")
	for {
		select {
		case record, ok := <-logs.Chan():
			if !ok {
				if err := logs.Error(); err != nil {
					fmt.Printf("Error reading logs: %s\n", status.Convert(err).Message())
					os.Exit(1)
				}
				return nil
			}
			switch output {
			case "json":
				b, _ := json.Marshal(record)
				fmt.Printf("%v\n", string(b))
			default:
				fmt.Printf("%v\n", record.Message)

			}
		}
	}
}

func humanizeStatus(status runtime.ServiceStatus) string {
	switch status {
	case runtime.Pending:
		return "pending"
	case runtime.Building:
		return "building"
	case runtime.Starting:
		return "starting"
	case runtime.Running:
		return "running"
	case runtime.Stopping:
		return "stopping"
	case runtime.Stopped:
		return "stopped"
	case runtime.Error:
		return "error"
	default:
		return "unknown"
	}
}
