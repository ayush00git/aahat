package data

import (
	"context"
	"errors"
	"io"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Store reads keys from s3://bucket/prefix/.
type S3Store struct {
	client *s3.Client
	bucket string
	prefix string
}

func NewS3Store(client *s3.Client, bucket, prefix string) *S3Store {
	return &S3Store{client: client, bucket: bucket, prefix: prefix}
}

func (s *S3Store) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path.Join(s.prefix, key)),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}
