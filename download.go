package b2

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DownloadFileByID gets file contents by file ID. The ReadCloser must be
// closed by the caller once done reading.
//
// Note: the (*FileInfo).CustomMetadata values returned by this function are
// all represented as strings, because they are delivered by HTTP headers.
func (c *Client) DownloadFileByID(ctx context.Context, id string) (io.ReadCloser, *FileInfo, error) {
	downloadURL := c.loginInfo.Load().(*LoginInfo).DownloadURL
	U := downloadURL + apiPath + "b2_download_file_by_id?fileId=" + id
	req, err := http.NewRequestWithContext(ctx, "GET", U, nil)
	if err != nil {
		return nil, nil, err
	}
	res, err := c.hc.Do(req)
	if e, ok := UnwrapError(err); ok && e.Status == http.StatusUnauthorized {
		if err = c.login(ctx, res); err == nil {
			req, err := http.NewRequestWithContext(ctx, "GET", U, nil)
			if err != nil {
				return nil, nil, err
			}
			res, err = c.hc.Do(req)
		}
	}
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
	req, err := http.NewRequestWithContext(ctx, "GET", U, nil)
	if err != nil {
		return nil, nil, err
	}
	res, err := c.hc.Do(req)
	if e, ok := UnwrapError(err); ok && e.Status == http.StatusUnauthorized {
		if err = c.login(ctx, res); err == nil {
			req, err := http.NewRequestWithContext(ctx, "GET", U, nil)
			if err != nil {
				return nil, nil, err
			}
			res, err = c.hc.Do(req)
		}
	}
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
