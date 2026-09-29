// This code is copypasted from the tonutils-storage-provider package and modified.
// Original package:
// https://github.com/xssnick/tonutils-storage-provider
// According to the license, this code is licensed under the Apache License 2.0
// See the LICENSE file in the original package for more details.
package tonstorage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	defaultTimeout = 15 * time.Second
	createTimeout  = 2 * time.Minute
)

type Client interface {
	Create(ctx context.Context, description, path string) (string, error)
	GetBag(ctx context.Context, bagId string) (*BagDetailed, error)
	List(ctx context.Context) (*ListShort, error)
	StartDownload(ctx context.Context, bagId string, downloadAll bool) error
	RemoveBag(ctx context.Context, bagId string, withFiles bool) error
}

type client struct {
	base         string
	rootPath     string
	client       http.Client
	createClient http.Client
	credentials  *Credentials
}

type Credentials struct {
	Login    string
	Password string
}

var ErrNotFound = errors.New("not found")

func (c *client) Create(ctx context.Context, description, path string) (string, error) {
	type request struct {
		Description string `json:"description"`
		Path        string `json:"path"`
	}

	var res struct {
		BagID string `json:"bag_id"`
	}

	if err := c.doRequest(ctx, &c.createClient, "POST", "/api/v1/create", request{
		Description: description,
		Path:        path,
	}, &res); err != nil {
		return "", fmt.Errorf("failed to do request: %w", err)
	}

	if res.BagID == "" {
		return "", fmt.Errorf("empty bag ID in response")
	}

	return res.BagID, nil
}

func (c *client) GetBag(ctx context.Context, bagId string) (*BagDetailed, error) {
	var res BagDetailed
	if err := c.doRequest(ctx, &c.client, "GET", "/api/v1/details?bag_id="+bagId, nil, &res); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("failed to do request: %w", err)
	}

	if res.InfoLoaded && res.MerkleHash == "" {
		return nil, fmt.Errorf("too old tonutils-storage version, please update")
	}
	return &res, nil
}

func (c *client) List(ctx context.Context) (*ListShort, error) {
	var res ListShort
	if err := c.doRequest(ctx, &c.client, "GET", "/api/v1/list", nil, &res); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("failed to do request: %w", err)
	}

	return &res, nil
}

func (c *client) StartDownload(ctx context.Context, bagId string, downloadAll bool) error {
	type request struct {
		BagID       string   `json:"bag_id"`
		Path        string   `json:"path"`
		DownloadAll bool     `json:"download_all"`
		Files       []uint32 `json:"files"`
	}

	var res Result
	if err := c.doRequest(ctx, &c.client, "POST", "/api/v1/add", request{
		BagID:       bagId,
		Path:        c.rootPath,
		DownloadAll: downloadAll,
	}, &res); err != nil {
		return fmt.Errorf("failed to do request: %w", err)
	}

	if !res.Ok {
		return fmt.Errorf("error in response: %s", res.Error)
	}
	return nil
}

func (c *client) RemoveBag(ctx context.Context, bagId string, withFiles bool) error {
	type request struct {
		BagID     string `json:"bag_id"`
		WithFiles bool   `json:"with_files"`
	}

	var res Result
	if err := c.doRequest(ctx, &c.client, "POST", "/api/v1/remove", request{
		BagID:     bagId,
		WithFiles: withFiles,
	}, &res); err != nil {
		return fmt.Errorf("failed to do request: %w", err)
	}

	if !res.Ok {
		return fmt.Errorf("error in response: %s", res.Error)
	}
	return nil
}

func (c *client) doRequest(ctx context.Context, httpClient *http.Client, method, url string, req, resp any) error {
	buf := &bytes.Buffer{}
	if req != nil {
		if err := json.NewEncoder(buf).Encode(req); err != nil {
			return fmt.Errorf("failed to encode request data: %w", err)
		}
	}

	r, err := http.NewRequestWithContext(ctx, method, c.base+url, buf)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	if c.credentials != nil {
		r.SetBasicAuth(c.credentials.Login, c.credentials.Password)
	}

	res, err := httpClient.Do(r)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == 404 {
		return ErrNotFound
	}

	if res.StatusCode != 200 {
		var e Result
		if err = json.NewDecoder(res.Body).Decode(&e); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
		return fmt.Errorf("status code is %d, error: %s", res.StatusCode, e.Error)
	}

	if err = json.NewDecoder(res.Body).Decode(resp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}

func NewClient(base, rootPath string, credentials *Credentials) Client {
	return &client{
		base:     base,
		rootPath: rootPath,
		client: http.Client{
			Timeout: defaultTimeout,
		},
		createClient: http.Client{
			Timeout: createTimeout,
		},
		credentials: credentials,
	}
}
