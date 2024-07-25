package storage

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/minio/minio-go/v7"
)

type UploadTarget struct {
	Base    string
	Root    string
	Problem Problem
}

type FileInfo struct {
	base     string
	path     string
	required bool
}

func NewUploadTarget(base, root string) (UploadTarget, error) {
	h, err := testCaseHash(base)
	if err != nil {
		return UploadTarget{}, err
	}
	v, err := version(base, root)
	if err != nil {
		return UploadTarget{}, err
	}
	return UploadTarget{
		Root: root,
		Base: base,
		Problem: Problem{
			Name:         path.Base(base),
			TestCaseHash: h,
			Version:      v,
		},
	}, nil
}

func testCaseHash(base string) (string, error) {
	caseHash, err := os.ReadFile(path.Join(base, "hash.json"))
	if err != nil {
		return "", err
	}
	var cases map[string]string
	if err := json.Unmarshal(caseHash, &cases); err != nil {
		return "", err
	}

	hashes := make([]string, 0, len(cases))
	for _, v := range cases {
		hashes = append(hashes, v)
	}
	return joinHashes(hashes), nil
}

func version(base, root string) (string, error) {
	hashes := []string{}

	if h, err := testCaseHash(base); err != nil {
		return "", err
	} else {
		hashes = append(hashes, h)
	}

	for _, info := range fileInfos(base, root) {
		path := path.Join(info.base, info.path)
		h, err := fileHash(path)
		if info.required && err != nil {
			return "", err
		}
		hashes = append(hashes, h)
	}

	return joinHashes(hashes), nil
}

func fileInfos(base, root string) []FileInfo {
	return []FileInfo{
		// Common files
		// TODO: stop to manually add all common/*.h
		{
			base:     root,
			path:     path.Join("common", "fastio.h"),
			required: true,
		},
		{
			base:     root,
			path:     path.Join("common", "random.h"),
			required: true,
		},
		{
			base:     root,
			path:     path.Join("common", "testlib.h"),
			required: true,
		},
		// Problem files
		{
			base:     base,
			path:     path.Join("task.md"),
			required: true,
		},
		{
			base:     base,
			path:     path.Join("info.toml"),
			required: true,
		},
		{
			base:     base,
			path:     path.Join("checker.cpp"),
			required: true,
		},
		{
			base:     base,
			path:     path.Join("params.h"),
			required: true,
		},
		// for C++(Function)
		{
			base:     base,
			path:     path.Join("grader", "grader.cpp"),
			required: false,
		},
		{
			base:     base,
			path:     path.Join("grader", "solve.hpp"),
			required: false,
		},
	}
}

func (p UploadTarget) UploadTestcases(client Client) error {
	h := p.Problem.TestCaseHash
	v := p.Problem.Version

	tempFile, err := os.CreateTemp("", "testcase*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	gzipWriter := gzip.NewWriter(tempFile)
	tarWriter := tar.NewWriter(gzipWriter)

	for _, ext := range []string{"in", "out"} {
		if err := filepath.Walk(path.Join(p.Base, ext), func(fpath string, info fs.FileInfo, err error) error {
			if strings.Contains(fpath, "example") {
				if _, err := client.client.FPutObject(context.Background(), client.publicBucket, fmt.Sprintf("v2/%s/%s/%s/%s", p.Problem.Name, v, ext, path.Base(fpath)), fpath, minio.PutObjectOptions{}); err != nil {
					return err
				}
			}

			if path.Ext(fpath) == fmt.Sprintf(".%s", ext) {
				file, err := os.Open(fpath)
				if err != nil {
					return err
				}
				defer file.Close()

				fileInfo, err := file.Stat()
				if err != nil {
					return err
				}

				header := &tar.Header{
					Name: fmt.Sprintf("%s/%s", ext, filepath.Base(fpath)),
					Size: fileInfo.Size(),
					Mode: 0600,
				}

				if err := tarWriter.WriteHeader(header); err != nil {
					return err
				}

				_, err = io.Copy(tarWriter, file)
				if err != nil {
					return err
				}

				return nil
			}
			return nil
		}); err != nil {
			return err
		}
	}

	if err := tarWriter.Close(); err != nil {
		return err
	}
	if err := gzipWriter.Close(); err != nil {
		return err
	}

	if _, err := tempFile.Seek(0, 0); err != nil {
		return err
	}
	fileInfo, err := tempFile.Stat()
	if err != nil {
		return err
	}

	if _, err := client.client.PutObject(context.Background(), client.bucket, fmt.Sprintf("v2/%s/%s.tar.gz", p.Problem.Name, h), tempFile, fileInfo.Size(), minio.PutObjectOptions{}); err != nil {
		return err
	}

	return nil
}

func (p UploadTarget) UploadFiles(client Client) error {
	v := p.Problem.Version

	for _, info := range fileInfos(p.Base, p.Root) {
		src := path.Join(info.base, info.path)
		if _, err := os.Stat(src); err != nil {
			if info.required {
				return fmt.Errorf("required file: %s/%s not found", info.base, info.path)
			}
			continue
		}

		if _, err := client.client.FPutObject(context.Background(), client.publicBucket, fmt.Sprintf("v2/%s/%s/%s", p.Problem.Name, v, info.path), src, minio.PutObjectOptions{}); err != nil {
			return err
		}
	}

	return nil
}

func fileHash(path string) (string, error) {
	checker, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(checker)), nil
}

func joinHashes(hashes []string) string {
	arr := make([]string, len(hashes))
	copy(arr, hashes)
	sort.Strings(arr)

	h := sha256.New()
	for _, v := range arr {
		h.Write([]byte(v))
	}
	return fmt.Sprintf("%x", h.Sum([]byte{}))
}
