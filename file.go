package b2

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// DeleteFile deletes a file version.
func (c *Client) DeleteFile(ctx context.Context, id, name string) error {
	res, err := c.doRequest(ctx, "b2_delete_file_version", map[string]interface{}{
		"fileId": id, "fileName": name,
	})
	if err != nil {
		return err
	}
	drainAndClose(res.Body)
	return nil
}

// A FileInfo is the metadata associated with a specific file version.
type FileInfo struct {
	ID   string
	Name string

	// Had to remove BucketID since it is not returned by b2_download_file_by_*
	// BucketID string

	ContentSHA1   string // hex encoded
	ContentLength int64
	ContentType   string

	CustomMetadata  map[string]interface{}
	UploadTimestamp time.Time

	// If Action is "hide", this ID does not refer to a file version
	// but to an hiding action. Otherwise "upload".
	Action string
}

type fileInfoObj struct {
	AccountID       string                 `json:"accountId"`
	BucketID        string                 `json:"bucketId"`
	ContentLength   int64                  `json:"contentLength"`
	ContentSHA1     string                 `json:"contentSha1"`
	ContentType     string                 `json:"contentType"`
	FileID          string                 `json:"fileId"`
	FileInfo        map[string]interface{} `json:"fileInfo"`
	FileName        string                 `json:"fileName"`
	UploadTimestamp int64                  `json:"uploadTimestamp"`
	Action          string                 `json:"action"`
}

func (fi *fileInfoObj) makeFileInfo() *FileInfo {
	return &FileInfo{
		ID:              fi.FileID,
		Name:            fi.FileName,
		ContentLength:   fi.ContentLength,
		ContentSHA1:     fi.ContentSHA1,
		ContentType:     fi.ContentType,
		CustomMetadata:  fi.FileInfo,
		Action:          fi.Action,
		UploadTimestamp: time.Unix(fi.UploadTimestamp/1e3, fi.UploadTimestamp%1e3*1e6),
	}
}

// GetFileInfoByID obtains a FileInfo for a given ID.
//
// The ID can refer to any file version or "hide" action in any bucket.
func (c *Client) GetFileInfoByID(ctx context.Context, id string) (*FileInfo, error) {
	res, err := c.doRequest(ctx, "b2_get_file_info", map[string]interface{}{
		"fileId": id,
	})
	if err != nil {
		return nil, err
	}
	defer drainAndClose(res.Body)
	var fi *fileInfoObj
	if err := json.NewDecoder(res.Body).Decode(&fi); err != nil {
		return nil, err
	}
	return fi.makeFileInfo(), nil
}

var FileNotFoundError = errors.New("no file with the given name in the bucket")

// GetFileInfoByName obtains a FileInfo for a given name.
//
// If the file doesn't exist, FileNotFoundError is returned.
// If multiple versions of the file exist, only the latest is returned.
func (b *Bucket) GetFileInfoByName(ctx context.Context, name string) (*FileInfo, error) {
	l := b.ListFiles(ctx, name)
	l.SetPageCount(1)
	if l.Next() {
		if l.FileInfo().Name == name {
			return l.FileInfo(), nil
		}
	}
	if err := l.Err(); err != nil {
		return nil, l.Err()
	}
	return nil, FileNotFoundError
}

// A Listing is the result of (*Bucket).ListFiles[Versions].
// It works like sql.Rows: use Next to advance and then FileInfo.
// Check Err once Next returns false.
//
//	l := b.ListFiles("")
//	for l.Next() {
//	    fi := l.FileInfo()
//	    ...
//	}
//	if err := l.Err(); err != nil {
//	    ...
//	}
//
// A Listing handles pagination transparently, so it iterates until
// the last file in the bucket. To limit the number of results, do this.
//
//	for i := 0; i < limit && l.Next(); i++ {
type Listing struct {
	ctx              context.Context
	b                *Bucket
	versions         bool
	nextPageCount    int
	nextName, nextID *string
	objects          []*FileInfo // in reverse order
	err              error
}

// SetPageCount controls the number of results to be fetched with each API
// call. The maximum n is 1000, higher values are automatically limited to 1000.
//
// SetPageCount does not limit the number of results returned by a Listing.
func (l *Listing) SetPageCount(n int) {
	if n > 1000 {
		n = 1000
	}
	l.nextPageCount = n
}

// Next calls the list API if needed and prepares the FileInfo results.
// It returns true on success, or false if there is no next result
// or an error happened while preparing it. Err should be
// consulted to distinguish between the two cases.
func (l *Listing) Next() bool {
	if l.err != nil {
		return false
	}
	if len(l.objects) > 0 {
		l.objects = l.objects[:len(l.objects)-1]
	}
	if len(l.objects) > 0 {
		return true
	}
	if l.nextName == nil {
		return false // end of iteration
	}

	data := map[string]interface{}{
		"bucketId":      l.b.ID,
		"startFileName": *l.nextName,
		"maxFileCount":  l.nextPageCount,
	}
	endpoint := "b2_list_file_names"
	if l.versions {
		endpoint = "b2_list_file_versions"
	}
	if l.nextID != nil && *l.nextID != "" {
		data["startFileId"] = *l.nextID
	}
	r, err := l.b.c.doRequest(l.ctx, endpoint, data)
	if err != nil {
		l.err = err
		return false
	}
	defer drainAndClose(r.Body)

	var x struct {
		Files        []fileInfoObj
		NextFileName *string
		NextFileID   *string
	}
	if l.err = json.NewDecoder(r.Body).Decode(&x); l.err != nil {
		return false
	}

	l.objects = make([]*FileInfo, len(x.Files))
	for i, f := range x.Files {
		l.objects[len(l.objects)-1-i] = f.makeFileInfo()
	}
	l.nextName, l.nextID = x.NextFileName, x.NextFileID
	return len(l.objects) > 0
}

// FileInfo returns the FileInfo object made available by Next.
//
// FileInfo must only be called after a call to Next returned true.
func (l *Listing) FileInfo() *FileInfo {
	return l.objects[len(l.objects)-1]
}

// Err returns the error, if any, that was encountered while listing.
func (l *Listing) Err() error {
	return l.err
}

// ListFiles returns a Listing of files in the Bucket, alphabetically sorted,
// starting from the file named fromName (included if it exists). To start from
// the first file in the bucket, set fileName to "".
//
// ListFiles only returns the most recent version of each (non-hidden) file.
// If you want to fetch all versions, use ListFilesVersions.
func (b *Bucket) ListFiles(ctx context.Context, fromName string) *Listing {
	return &Listing{
		ctx:      ctx,
		b:        b,
		nextName: &fromName,
	}
}

// ListFilesVersions is like ListFiles, but returns all file versions,
// alphabetically sorted first, and by reverse of date/time uploaded then.
//
// If fromID is specified, the name-and-id pair is the starting point.
func (b *Bucket) ListFilesVersions(ctx context.Context, fromName, fromID string) *Listing {
	if fromName == "" && fromID != "" {
		return &Listing{
			err: errors.New("can't set fromID if fromName is not set"),
		}
	}
	return &Listing{
		ctx:      ctx,
		b:        b,
		versions: true,
		nextName: &fromName,
		nextID:   &fromID,
	}
}
