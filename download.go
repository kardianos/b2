package b2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (c *Client) getWithAuth(ctx context.Context, U string, Range string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", U, nil)
	if err != nil {
		return nil, err
	}
	if len(Range) > 0 {
		req.Header.Set("Range", Range)
	}
	res, err := c.hc.Do(req)
	if e, ok := UnwrapError(err); ok && e.Status == http.StatusUnauthorized {
		if err = c.login(ctx, res); err == nil {
			req, err = http.NewRequestWithContext(ctx, "GET", U, nil)
			if err != nil {
				return nil, err
			}
			return c.hc.Do(req)
		}
	}
	return res, err
}

type Range struct {
	Begin int64
	End   int64
}

type DownloadOptions struct {
	// Download file by ID.
	FileID string

	// Download file by bucket and filename.
	Bucket   string
	FileName string

	// Zero based indicies.
	Range Range
}

// DownloadFile gets file contents. The ReadCloser must be
// closed by the caller once done reading.
//
// Note: the (*FileInfo).CustomMetadata values returned by this function are
// all represented as strings, because they are delivered by HTTP headers.
func (c *Client) DownloadFile(ctx context.Context, o DownloadOptions) (io.ReadCloser, *FileInfo, error) {
	downloadURL := c.loginInfo.Load().(*LoginInfo).DownloadURL
	var U string
	switch {
	default:
		return nil, nil, errors.New("must specify a file name or file ID")
	case len(o.FileID) > 0:
		U = downloadURL + apiPath + "b2_download_file_by_id?fileId=" + o.FileID
	case len(o.FileName) > 0:
		if len(o.Bucket) == 0 {
			return nil, nil, errors.New("empty bucket name, required when using FileName")
		}
		U = downloadURL + "/file/" + o.Bucket + "/" + o.FileName
	}
	var rs string
	if r := o.Range; r.Begin > 0 || r.End > 0 {
		if r.Begin < 0 {
			r.Begin = 0
		}
		if r.End < 1 {
			return nil, nil, fmt.Errorf("invalid range end %d, must be greater then 0", r.End)
		}
		rs = fmt.Sprintf("bytes=%d-%d", r.Begin, r.End)
	}
	res, err := c.getWithAuth(ctx, U, rs)
	if err != nil {
		debugf("download %s: %s", U, err)
		return nil, nil, err
	}
	debugf("download %s (%s)", U, res.Header.Get("X-Bz-Content-Sha1"))

	fi, err := parseFileInfoHeaders(res.Header)
	return res.Body, fi, err
}

// DownloadFileByID gets file contents by file ID. The ReadCloser must be
// closed by the caller once done reading.
//
// Note: the (*FileInfo).CustomMetadata values returned by this function are
// all represented as strings, because they are delivered by HTTP headers.
func (c *Client) DownloadFileByID(ctx context.Context, id string) (io.ReadCloser, *FileInfo, error) {
	downloadURL := c.loginInfo.Load().(*LoginInfo).DownloadURL
	U := downloadURL + apiPath + "b2_download_file_by_id?fileId=" + id
	res, err := c.getWithAuth(ctx, U, "")
	if err != nil {
		debugf("download %s: %s", id, err)
		return nil, nil, err
	}
	debugf("download %s (%s)", id, res.Header.Get("X-Bz-Content-Sha1"))

	fi, err := parseFileInfoHeaders(res.Header)
	return res.Body, fi, err
}

// DownloadFileByName gets file contents by file and bucket name.
// The ReadCloser must be closed by the caller once done reading.
//
// Note: the (*FileInfo).CustomMetadata values returned by this function are
// all represented as strings, because they are delivered by HTTP headers.
func (c *Client) DownloadFileByName(ctx context.Context, bucket, file string) (io.ReadCloser, *FileInfo, error) {
	downloadURL := c.loginInfo.Load().(*LoginInfo).DownloadURL
	U := downloadURL + "/file/" + bucket + "/" + file
	res, err := c.getWithAuth(ctx, U, "")
	if err != nil {
		debugf("download %s: %s", file, err)
		return nil, nil, err
	}
	debugf("download %s (%s)", file, res.Header.Get("X-Bz-Content-Sha1"))

	fi, err := parseFileInfoHeaders(res.Header)
	return res.Body, fi, err
}

func parseFileInfoHeaders(h http.Header) (*FileInfo, error) {
	fi := &FileInfo{
		ID:          h.Get("X-Bz-File-Id"),
		Name:        h.Get("X-Bz-File-Name"),
		ContentType: h.Get("Content-Type"),
		ContentSHA1: h.Get("X-Bz-Content-Sha1"),
		Action:      "upload",
	}
	timestamp, err := strconv.ParseInt(h.Get("X-Bz-Upload-Timestamp"), 10, 64)
	if err != nil {
		return nil, err
	}
	fi.UploadTimestamp = time.Unix(timestamp/1e3, timestamp%1e3*1e6)
	fi.ContentLength, err = strconv.ParseInt(h.Get("Content-Length"), 10, 64)
	if err != nil {
		return nil, err
	}

	fi.CustomMetadata = make(map[string]string)
	for name := range h {
		if !strings.HasPrefix(name, "X-Bz-Info-") {
			continue
		}
		fi.CustomMetadata[name[len("X-Bz-Info-"):]] = h.Get(name)
	}

	return fi, nil
}
