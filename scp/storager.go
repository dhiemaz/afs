package scp

import (
	"bytes"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/viant/afs/file"
	"github.com/viant/afs/option"
	"github.com/viant/afs/storage"
	"golang.org/x/crypto/ssh"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"
)

type storager struct {
	address string
	*ssh.ClientConfig
	*ssh.Client
	timeout time.Duration
}

func (s *storager) connect() (err error) {
	if s.Client, err = ssh.Dial("tcp", s.address, s.ClientConfig); err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to dial %s", s.address))
	}
	return nil
}

//Delete removes supplied asset
func (s *storager) Delete(ctx context.Context, location string) error {
	session, err := s.NewSession()
	if err == nil {
		_, err = session.Output(fmt.Sprintf("rm -rf %v", location))
	}
	return err

}

//Exists returns true if location exists
func (s *storager) Exists(ctx context.Context, location string) (bool, error) {
	session, err := newSession(s.Client, modeRead, true, s.timeout)
	if err != nil {
		return false, err
	}
	location = path.Clean(location)
	has := false
	_ = session.download(ctx, false, location, func(parent string, info os.FileInfo, reader io.Reader) (b bool, e error) {
		has = true
		return false, nil
	})
	return has, nil
}

//List lists location assets
func (s *storager) List(ctx context.Context, location string, options ...storage.Option) ([]os.FileInfo, error) {
	page := &option.Page{}
	var matcher option.Matcher
	option.Assign(options, &page, &matcher)
	matcher = option.GetMatcher(matcher)
	var result = make([]os.FileInfo, 0)
	err := s.Walk(ctx, location, func(relative string, info os.FileInfo, reader io.Reader) (shaleContinue bool, err error) {
		if !matcher(relative, info) {
			return true, nil
		}
		page.Increment()
		if page.ShallSkip() {
			return true, nil
		}
		result = append(result, info)
		return !page.HasReachedLimit(), nil
	})

	return result, err
}

//Walk visits location resources
func (s *storager) Walk(ctx context.Context, location string, handler func(relative string, info os.FileInfo, reader io.Reader) (bool, error), options ...storage.Option) error {
	session, err := newSession(s.Client, modeRead, true, s.timeout)
	if err != nil {
		return err
	}
	location = path.Clean(location)
	return session.download(ctx, true, location, handler)
}

//Download fetches content for supplied location
func (s *storager) Download(ctx context.Context, location string, options ...storage.Option) (io.ReadCloser, error) {
	result := new(bytes.Buffer)
	err := s.Walk(ctx, location, func(relative string, info os.FileInfo, reader io.Reader) (b bool, e error) {
		_, err := io.Copy(result, reader)
		return false, err
	})
	return ioutil.NopCloser(result), err
}

//Uploader return batch uploader
func (s *storager) Uploader(ctx context.Context, destination string) (storage.Upload, io.Closer, error) {
	session, err := newSession(s.Client, modeWrite, true, 0)
	if err != nil {
		return nil, nil, err
	}
	return session.upload(destination)
}

//Upload uploads content for supplied destination
func (s *storager) Upload(ctx context.Context, destination string, mode os.FileMode, content []byte, options ...storage.Option) error {
	return s.Create(ctx, destination, mode, content, false)
}

//Create creates a file or directory
func (s *storager) Create(ctx context.Context, destination string, mode os.FileMode, content []byte, isDir bool, options ...storage.Option) error {
	parent, name := path.Split(destination)
	if isDir {
		if session, err := s.NewSession(); err == nil {
			if _, err := session.Output(fmt.Sprintf("mkdir -p %s", destination)); err == nil {
				return nil
			}
		}
	}
	upload, closer, err := s.Uploader(ctx, parent)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()
	info := file.NewInfo(name, int64(len(content)), mode, time.Now(), isDir)
	return upload(ctx, "", info, bytes.NewReader(content))
}

//NewStorager returns a new storager
func NewStorager(address string, timeout time.Duration, config *ssh.ClientConfig) (storage.Storager, error) {
	if !strings.Contains(address, ":") {
		address += fmt.Sprintf(":%d", DefaultPort)
	}
	result := &storager{
		address:      address,
		ClientConfig: config,
		timeout:      timeout,
	}
	return result, result.connect()
}
