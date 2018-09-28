package s6

import (
	"fmt"
)

type S3File struct {
	Region string
	Bucket string
	Key    string
}

func (f S3File) URL() string {
	return fmt.Sprintf("https://s3-%s.amazonaws.com/%s/%s", f.Region, f.Bucket, f.Key)
}
