# b2

Efficient, idiomatic Go library for Backblaze B2 Cloud Storage.

TODO:
 * [ ] Start large file upload: b2_start_large_file.
 * [ ] b2_get_upload_part_url
 * [ ] Upload to large file part: b2_upload_part.
 * [ ] Finish large file: b2_finish_large_file.
 * [x] Download range of file.
 * [x] List files with prefix.

## Plan to support large file upload

Large File
	takes existing sha1
	look for existing large file
		if exists
			look for existing parts
		if not exists
			start large file
	status and list of parts remaining

Large Status
	upload each part, if the upload URL doesn't exist or if it is invalid, get a new upload URL.
	when done, call complete


type LocalLargeFile interface {
	SHA1() string
	Size() int64
	RangeReader(r Range) (io.Reader, error)
}

type CreateLargeFileOptions struct {
	SHA1 string
	ModUnixMilli int64
	ContentType string
}

type LargeFile struct {
	Local io.ReadSizeSeekCloser
	FileID string
	SHA1 string
	Size int64
}

type LargePart struct {
	Index int
	Range Range
	SHA1 string
}


func UploadLargeFile(ctx, remotePath, LocalLargeFile) -> (LargeStatus, error)
func NewPartUploader() -> *PartUploader
func (LargeStatus) UploadPart(ctx, part, PartUploader) -> error
func (LargeStatus) Finish(ctx) -> error

