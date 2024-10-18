//go:build !solution

package artifact

import (
	"context"
	"fmt"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/tarstream"
	"net/http"

	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
)

// Download artifact from remote cache into local cache.
func Download(ctx context.Context, endpoint string, c *Cache, artifactID build.ID) error {
	severURL := fmt.Sprintf("%s/artifact?id=%s", endpoint, artifactID.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, severURL, nil)
	if err != nil {
		return err
	}

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	path, commit, abort, err := c.Create(artifactID)
	if err != nil {
		return err
	}

	err = tarstream.Receive(path, response.Body)
	if err != nil {
		abort()
		return err
	}

	commit()
	return nil
}
