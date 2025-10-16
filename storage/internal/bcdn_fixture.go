// SPDX-License-Identifier: ice License 1.0

//go:build test

package internal

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gookit/assert"
	"github.com/stretchr/testify/require"
)

func (c *client) CdnDownloadURL(filename string) string {
	if strings.HasPrefix(filename, c.config.URLDownload) {
		return filename
	}
	u, _ := url.JoinPath(c.config.URLDownload, filename)

	return u
}

func VerifyFileOnCdn(tb *testing.T, ctx context.Context, cdnClient CDNClient, fileName string) {
	tb.Helper()
	url := cdnClient.(*client).CdnDownloadURL(fileName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	require.NoError(tb, err)
	httpClient := http.DefaultClient
	resp, err := httpClient.Do(req)
	defer func() {
		require.NoError(tb, resp.Body.Close())
	}()
	require.NoError(tb, err)
	assert.Equal(tb, http.StatusOK, resp.StatusCode, url)
	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(tb, err)
	assert.NotEmpty(tb, bodyBytes)
}
func VerifyFileDeletedOnCdn(tb *testing.T, ctx context.Context, cdnClient CDNClient, fileName string) {
	tb.Helper()
	url := cdnClient.(*client).CdnDownloadURL(fileName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	require.NoError(tb, err)
	httpClient := http.DefaultClient
	resp, err := httpClient.Do(req)
	defer func() {
		require.NoError(tb, resp.Body.Close())
	}()
	require.NoError(tb, err)
	assert.Equal(tb, http.StatusNotFound, resp.StatusCode, url)
}
