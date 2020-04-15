package handler

import (
	"testing"
)

type parseCase struct {
	url      string
	expected *parsedGithubURL
}

func TestGithubExtract(t *testing.T) {
	cases := []parseCase{
		{
			url: "helloworld",
			expected: &parsedGithubURL{
				repoAddress: "https://github.com/micro/services",
				folder:      "helloworld",
			},
		},
		{
			url: "https://github.com/micro/services/tree/master/helloworld",
			expected: &parsedGithubURL{
				repoAddress: "https://github.com/micro/services",
				folder:      "helloworld",
			},
		},
	}
	for i, c := range cases {
		result, err := parseGithubURL(c.url)
		if err != nil {
			t.Fatalf("Failed case %v: %v", i, err)
		}
		if result.folder != c.expected.folder {
			t.Fatalf("Folder does not match for %v, expected %v, got %v", i, c.expected.folder, result.folder)
		}
		if result.repoAddress != c.expected.repoAddress {
			t.Fatalf("Repo address does not match for %v, expected %v, got %v", i, c.expected.repoAddress, result.repoAddress)
		}
	}
}

type nameCase struct {
	fileContent string
	expected    string
}

func TestServiceNameExtract(t *testing.T) {
	cases := []nameCase{
		{
			fileContent: `func main() {
			// New Service
			service := micro.NewService(
				micro.Name("go.micro.service.helloworld"),
				micro.Version("latest"),
			)`,
			expected: "go.micro.service.helloworld",
		},
	}
	for i, c := range cases {
		result := extractServiceName([]byte(c.fileContent))
		if result != c.expected {
			t.Fatalf("Case %v, expected: %v, got: %v", i, c.expected, result)
		}
	}
}
