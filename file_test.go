package b2_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/kardianos/b2"
)

func getBucket(t *testing.T, ctx context.Context, c *b2.Client) *b2.BucketInfo {
	r := make([]byte, 6)
	rand.Read(r)
	name := "test-" + hex.EncodeToString(r)

	b, err := c.CreateBucket(ctx, name, false)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

func deleteBucket(t *testing.T, b *b2.BucketInfo) {
	ctx := context.Background()
	if err := b.Delete(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestFileLifecycle(t *testing.T) {
	ctx := context.Background()
	c := getClient(t, ctx)
	b := getBucket(t, ctx, c)
	defer deleteBucket(t, b)

	const fSize = 123456
	file := make([]byte, fSize)
	rand.Read(file)
	fiu, err := b.Upload(ctx, bytes.NewReader(file), "test-foo", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	fi, err := c.GetFileInfoByID(ctx, fiu.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fi.ID != fiu.ID {
		t.Error("Mismatched file ID")
	}
	if fi.ContentLength != fSize {
		t.Error("Mismatched file length")
	}
	if fi.Name != "test-foo" {
		t.Error("Mismatched file name")
	}
	if fi.UploadTimestamp.After(time.Now()) || fi.UploadTimestamp.Before(time.Now().Add(-time.Hour)) {
		t.Error("Wrong UploadTimestamp")
	}
	if fi.ContentSHA1 != fiu.ContentSHA1 {
		t.Error("Mismatched SHA1")
	}
	digest := sha1.Sum(file)
	if fi.ContentSHA1 != hex.EncodeToString(digest[:]) {
		t.Error("Wrong SHA1")
	}
	{
		rc, fi, err := c.DownloadFile(ctx, b2.DownloadOptions{
			FileID: fi.ID,
			Range: b2.Range{
				Begin: 2,
				End:   3,
			},
		})
		if err != nil {
			t.Fatalf("download file: %v", err)
		}
		if fi.ContentLength != 2 {
			t.Fatalf("expected content length of 2, got %d", fi.ContentLength)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("failed to read: %v", err)
		}
		if g, w := body, file[2:4]; !bytes.Equal(g, w) {
			t.Fatalf("file download incorrect: want %v got %v", w, g)
		}
	}

	fi, err = b.GetFileInfoByName(ctx, "test-foo")
	if err != nil {
		t.Fatal(err)
	}
	if fi.ID != fiu.ID {
		t.Error("Mismatched file ID in GetByName")
	}
	_, err = b.GetFileInfoByName(ctx, "not-exists")
	if err != b2.ErrFileNotFound {
		t.Errorf("b.GetFileInfoByName did not return FileNotFoundError: %v", err)
	}

	rc, fi2, err := c.DownloadFileByID(ctx, fiu.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fi2.UploadTimestamp != fi.UploadTimestamp {
		t.Error("mismatch in c.DownloadFileByID -> fi.UploadTimestamp")
	}
	if fi2.ContentSHA1 != fi.ContentSHA1 {
		t.Error("mismatch in c.DownloadFileByID -> fi.ContentSHA1")
	}
	if fi2.ContentLength != fSize {
		t.Error("mismatch in c.DownloadFileByID -> fi.ContentLength")
	}
	body, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, file) {
		t.Error("mismatch in file contents")
	}
	rc.Close()

	rc, fi3, err := c.DownloadFileByName(ctx, b.Name, "test-foo")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fi2, fi3) {
		t.Error("DownloadFileByID.FileInfo != DownloadFileByName.FileInfo")
	}
	body, err = io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, file) {
		t.Error("mismatch in file contents")
	}
	rc.Close()

	if err := c.DeleteFile(ctx, fiu.ID, "test-foo"); err != nil {
		t.Fatal(err)
	}
}

func TestFileListing(t *testing.T) {
	ctx := context.Background()
	c := getClient(t, ctx)
	b := getBucket(t, ctx, c)
	defer deleteBucket(t, b)

	file := make([]byte, 1234)
	rand.Read(file)

	for i := 0; i < 2; i++ {
		fi, err := b.Upload(ctx, bytes.NewReader(file), "test-3", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.DeleteFile(ctx, fi.ID, fi.Name)
	}

	var fileIDs []string
	for i := 0; i < 5; i++ {
		fi, err := b.Upload(ctx, bytes.NewReader(file), fmt.Sprintf("test-%d", i), "", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.DeleteFile(ctx, fi.ID, fi.Name)
		fileIDs = append(fileIDs, fi.ID)
	}

	i, l := 0, b.ListFiles(ctx, b2.ListOptions{})
	for l.Next() {
		fi := l.FileInfo()
		if fi.ID != fileIDs[i] {
			t.Errorf("wrong file ID number %d: expected %s, got %s", i, fileIDs[i], fi.ID)
		}
		i++
	}
	if err := l.Err(); err != nil {
		t.Fatal(err)
	}
	if i != len(fileIDs) {
		t.Errorf("got %d files, expected %d", i-1, len(fileIDs)-1)
	}

	i, l = 1, b.ListFiles(ctx, b2.ListOptions{FromName: "test-1"})
	l.SetPageCount(3)
	for l.Next() {
		fi := l.FileInfo()
		if fi.ID != fileIDs[i] {
			t.Errorf("wrong file ID number %d: expected %s, got %s", i, fileIDs[i], fi.ID)
		}
		i++
	}
	if err := l.Err(); err != nil {
		t.Fatal(err)
	}
	if i != len(fileIDs) {
		t.Errorf("got %d files, expected %d", i-1, len(fileIDs)-1)
	}

	i, l = 0, b.ListFileVersions(ctx, b2.ListOptions{})
	l.SetPageCount(2)
	for l.Next() {
		i++
	}
	if err := l.Err(); err != nil {
		t.Fatal(err)
	}
	if i != len(fileIDs)+2 {
		t.Errorf("got %d files, expected %d", i-1, len(fileIDs)-1+2)
	}
}
