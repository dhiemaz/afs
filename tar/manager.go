package tar

import (
	"context"
	"fmt"
	"github.com/viant/afs/base"
	"github.com/viant/afs/option"
	"github.com/viant/afs/storage"
	"github.com/viant/afs/url"
	"io"
	"io/ioutil"
	"os"
)

type manager struct {
	*base.Manager
}

func (m *manager) provider(ctx context.Context, baseURL string, options ...storage.Option) (storage.Storager, error) {
	var manager storage.Manager
	option.Assign(options, &manager)
	options = m.Options(options)
	URL := url.SchemeExtensionURL(baseURL)
	if URL == "" {
		return nil, fmt.Errorf("extneded URL was empty: %v", baseURL)
	}
	if manager == nil {
		return nil, fmt.Errorf("manager for URL was empty: %v", URL)
	}
	return newStorager(ctx, baseURL, manager)
}

func (m *manager) Walk(ctx context.Context, URL string, handler storage.OnVisit, options ...storage.Option) error {
	baseURL, URLPath := url.Base(URL, Scheme)
	var matcher option.Matcher

	options, _ = option.Assign(options, &matcher)
	matcher = option.GetMatcher(matcher)
	srv, err := m.Storager(ctx, baseURL, options...)
	if err != nil {
		return err
	}
	service, ok := srv.(*storager)
	if !ok {
		return fmt.Errorf("unsupported storager type: expected: %T, but had %T", service, srv)
	}
	return service.Walk(ctx, URLPath, func(parent string, info os.FileInfo, reader io.Reader) (shallContinue bool, err error) {
		if !matcher(parent, info) {
			return true, nil
		}
		shallContinue, err = handler(ctx, baseURL, parent, info, ioutil.NopCloser(reader))
		return shallContinue, err
	}, options...)
}

func (m *manager) Uploader(ctx context.Context, URL string, options ...storage.Option) (storage.Upload, io.Closer, error) {
	_, URLPath := url.Base(URL, Scheme)
	srv, err := m.Storager(ctx, URL, options...)
	if err != nil {
		return nil, nil, err
	}
	service, ok := srv.(*storager)
	if !ok {
		return nil, nil, fmt.Errorf("unsupported storager type: expected: %T, but had %T", service, srv)
	}
	return service.Uploader(ctx, URLPath)
}

func newManager(options ...storage.Option) *manager {
	result := &manager{}
	baseMgr := base.New(result, Scheme, result.provider, options)
	result.Manager = baseMgr
	return result
}

//New creates zip manager
func New(options ...storage.Option) storage.Manager {
	return newManager(options...)
}
