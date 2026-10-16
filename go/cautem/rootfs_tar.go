package cautem

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"

	compatv1 "github.com/NVIDIA/OpenShell/sdk/go/proto/openshellv1"
)

// UploadRootfsTar creates an OpenShell staging slot and streams the rootfs tar
// to the gateway. The returned token belongs in the selected driver's
// rootfs_tar_staging_token configuration for CreateSandbox.
func (c *Client) UploadRootfsTar(ctx context.Context, fileName string, size uint64, source io.Reader) (string, error) {
	if strings.TrimSpace(fileName) == "" || strings.ContainsAny(fileName, `/\\`) || fileName == "." || fileName == ".." {
		return "", fmt.Errorf("upload rootfs tar: file name must be a base name")
	}
	if size == 0 || size > math.MaxInt64 || source == nil {
		return "", fmt.Errorf("upload rootfs tar: size and source are required")
	}
	rpc, err := c.compatClient()
	if err != nil {
		return "", err
	}
	staging, err := rpc.BeginRootfsTarStaging(ctx, &compatv1.BeginRootfsTarStagingRequest{
		WorkspaceScope: namedTemplateWorkspace(c.workspace()), FileName: fileName, SizeBytes: size,
	})
	if err != nil {
		return "", fmt.Errorf("allocate rootfs tar staging: %w", err)
	}
	endpoint, err := url.Parse(c.base)
	if err != nil {
		return "", fmt.Errorf("upload rootfs tar: invalid gateway URL: %w", err)
	}
	uploadURL := strings.TrimRight(endpoint.String(), "/") + "/v1/rootfs-tar/" + url.PathEscape(staging.GetStagingToken())
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, source)
	if err != nil {
		return "", fmt.Errorf("create rootfs tar upload request: %w", err)
	}
	request.ContentLength = int64(size)
	request.Header.Set("Content-Type", "application/x-tar")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	// Rootfs images can be much larger than ordinary control requests; context
	// cancellation bounds the stream instead of a short whole-request timeout.
	transport := c.http.HTTP.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return "", fmt.Errorf("upload rootfs tar: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("upload rootfs tar: gateway returned %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	return staging.GetStagingToken(), nil
}
