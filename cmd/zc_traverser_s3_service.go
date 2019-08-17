package cmd

import (
	"context"
	"net/url"
	"path/filepath"

	"github.com/minio/minio-go"

	"github.com/Azure/azure-storage-azcopy/common"
)

// As we discussed, the general architecture is that this is going to search a list of buckets and spawn s3Traversers for each bucket.
// This will modify the storedObject format a slight bit to add a "container" parameter.

type s3ServiceTraverser struct {
	ctx           context.Context
	bucketPattern string

	s3URL    s3URLPartsExtension
	s3Client *minio.Client

	// a generic function to notify that a new stored object has been enumerated
	incrementEnumerationCounter func()
}

func (t *s3ServiceTraverser) traverse(processor objectProcessor, filters []objectFilter) error {
	if bucketInfo, err := t.s3Client.ListBuckets(); err == nil {
		for _, v := range bucketInfo {
			// Match a pattern for the bucket name and the bucket name only
			if t.bucketPattern != "" {
				if ok, err := filepath.Match(t.bucketPattern, v.Name); err != nil {
					// Break if the pattern is invalid
					return err
				} else if !ok {
					// Ignore the bucket if it does not match the pattern
					continue
				}
			}

			tmpS3URL := t.s3URL
			tmpS3URL.BucketName = v.Name
			urlResult := tmpS3URL.URL()
			bucketTraverser, err := newS3Traverser(&urlResult, t.ctx, true, t.incrementEnumerationCounter)

			if err != nil {
				return err
			}

			middlemanProcessor := func(object storedObject) error {
				tmpObject := object
				tmpObject.containerName = v.Name

				return processor(tmpObject)
			}

			err = bucketTraverser.traverse(middlemanProcessor, filters)

			if err != nil {
				return err
			}
		}
	} else {
		return err
	}

	return nil
}

func newS3ServiceTraverser(rawURL *url.URL, ctx context.Context, incrementEnumerationCounter func()) (t *s3ServiceTraverser, err error) {
	t = &s3ServiceTraverser{ctx: ctx, incrementEnumerationCounter: incrementEnumerationCounter}

	var s3URLParts common.S3URLParts
	s3URLParts, err = common.NewS3URLParts(*rawURL)

	if err != nil {
		return
	} else if !s3URLParts.IsServiceSyntactically() {
		// Yoink the bucket name off and treat it as the pattern.
		t.bucketPattern = s3URLParts.BucketName

		s3URLParts.BucketName = ""
	}

	t.s3URL = s3URLPartsExtension{s3URLParts}

	t.s3Client, err = common.CreateS3Client(
		t.ctx,
		common.CredentialInfo{
			CredentialType: common.ECredentialType.S3AccessKey(),
			S3CredentialInfo: common.S3CredentialInfo{
				Endpoint: t.s3URL.Endpoint,
				Region:   t.s3URL.Region,
			},
		},
		common.CredentialOpOptions{
			LogError: glcm.Error,
		})

	return
}