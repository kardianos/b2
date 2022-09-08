package b2_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"io/ioutil"
	"os"
	"testing"
)

func TestUploadError(t *testing.T) {
	ctx := context.Background()
	c := getClient(t, ctx)
	b := getBucket(t, ctx, c)
	defer deleteBucket(t, b)

	file := make([]byte, 123456)
	rand.Read(file)
	_, err := b.Upload(ctx, bytes.NewReader(file), "illegal\x00filename", "", nil)
	if err == nil {
		t.Fatal("Expected an error")
	}
	t.Log(err)
}

func TestUploadFile(t *testing.T) {
	ctx := context.Background()
	c := getClient(t, ctx)
	b := getBucket(t, ctx, c)
	defer deleteBucket(t, b)

	tmpfile, err := ioutil.TempFile("", "b2")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	content := make([]byte, 123456)
	rand.Read(content)
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	fi, err := b.Upload(ctx, f, "foo-file", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.DeleteFile(ctx, fi.ID, fi.Name)
	if fi.ContentLength != 123456 {
		t.Error("mismatched fi.ContentLength", fi.ContentLength)
	}
	if n, err := io.Copy(ioutil.Discard, f); err != nil || n != 0 {
		t.Error("should have read 0 bytes:", n, err)
	}
}

func TestUploadBuffer(t *testing.T) {
	ctx := context.Background()
	c := getClient(t, ctx)
	b := getBucket(t, ctx, c)
	defer deleteBucket(t, b)

	content := make([]byte, 123456)
	rand.Read(content)
	buf := bytes.NewBuffer(content)
	fi, err := b.Upload(ctx, buf, "foo-file", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.DeleteFile(ctx, fi.ID, fi.Name)
	if fi.ContentLength != 123456 {
		t.Error("mismatched fi.ContentLength", fi.ContentLength)
	}
	if buf.Len() != 0 {
		t.Error("Buffer is not empty")
	}
}

func TestUploadReader(t *testing.T) {
	ctx := context.Background()
	c := getClient(t, ctx)
	b := getBucket(t, ctx, c)
	defer deleteBucket(t, b)

	content := make([]byte, 123456)
	rand.Read(content)
	r := bytes.NewReader(content)
	fi, err := b.Upload(ctx, ioutil.NopCloser(r), "foo-file", "", nil) // shadow Seek method
	if err != nil {
		t.Fatal(err)
	}
	defer c.DeleteFile(ctx, fi.ID, fi.Name)
	if fi.ContentLength != 123456 {
		t.Error("mismatched fi.ContentLength", fi.ContentLength)
	}
	if r.Len() != 0 {
		t.Error("Reader is not empty")
	}
}
