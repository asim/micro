package runtime

import (
	"io"

	goclient "github.com/micro/go-micro/v3/client"
	"github.com/micro/go-micro/v3/runtime/local/source/git"
	"github.com/micro/micro/v3/client/cli/namespace"
	cliutil "github.com/micro/micro/v3/client/cli/util"
	pb "github.com/micro/micro/v3/proto/runtime"
	"github.com/micro/micro/v3/service/client"
	"github.com/micro/micro/v3/service/context"
	"github.com/micro/micro/v3/service/runtime/util"
	"github.com/urfave/cli/v2"
)

const bufferSize = 1024

// upload source to the server. will return the source id, e.g. source://foo-bar and an error if
// one occured. The ID returned can be used as a source in runtime.Create.
func upload(ctx *cli.Context, source *git.Source) (string, error) {
	// if the source exists within a local git repository, archive the whole repository, otherwise
	// just archive the folder
	var tar io.Reader
	var err error
	if len(source.LocalRepoRoot) > 0 {
		tar, err = util.Archive(source.LocalRepoRoot)
	} else {
		tar, err = util.Archive(source.FullPath)
	}
	if err != nil {
		return "", err
	}

	// get the namespace of the client
	ns, err := namespace.Get(cliutil.GetEnv(ctx).Name)
	if err != nil {
		return "", err
	}

	// create an upload stream
	cli := pb.NewSourceService("runtime", client.DefaultClient)
	stream, err := cli.Upload(context.WithNamespace(ns), goclient.WithAuthToken())
	if err != nil {
		return "", err
	}

	// read bytes from the tar and stream it to the server
	buffer := make([]byte, bufferSize)
	for {
		num, err := tar.Read(buffer)
		if err == io.EOF {
			break
		} else if err != nil {
			return "", err
		}

		if err := stream.Send(&pb.UploadRequest{Data: buffer[:num]}); err != nil {
			return "", err
		}
	}

	// wait for the server to process the source
	rsp, err := stream.CloseAndRecv()
	if err != nil {
		return "", err
	}
	return rsp.Id, nil
}