package importables

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

const (
	ProviderPlausible       = "plausible"
	ProviderSimpleAnalytics = "simpleanalytics"
)

type SourceFile struct {
	ID        uuid.UUID
	Name      string
	Path      string
	SizeBytes int64
	SHA256    string
}

type SourceSet struct {
	Files      []SourceFile
	SourceHash string
	SiteDomain string
}

type TrafficRow struct {
	Date          time.Time
	Visitors      int64
	Visits        int64
	Pageviews     int64
	Bounces       int64
	VisitDuration int64
	SourceFile    string
}

type DimensionRow struct {
	Date          time.Time
	Dimension     string
	Name          string
	Detail        string
	Visitors      int64
	Visits        int64
	Pageviews     int64
	Bounces       int64
	VisitDuration int64
	Events        int64
	Entrances     int64
	Exits         int64
	SourceFile    string
}

type EventRow struct {
	Date       time.Time
	EventName  string
	Path       string
	LinkURL    string
	Visitors   int64
	Events     int64
	SourceFile string
}

type EventPropertyRow struct {
	Date          time.Time
	EventName     string
	PropertyKey   string
	PropertyValue string
	Visitors      int64
	Events        int64
	SourceFile    string
}

type EventDimensionRow struct {
	Date       time.Time
	EventName  string
	Dimension  string
	Name       string
	Detail     string
	Visitors   int64
	Events     int64
	SourceFile string
}

type Sink interface {
	PutTraffic(context.Context, TrafficRow) error
	PutDimension(context.Context, DimensionRow) error
	PutEvent(context.Context, EventRow) error
	PutEventProperty(context.Context, EventPropertyRow) error
	Flush(context.Context) error
}

type EventDimensionSink interface {
	PutEventDimension(context.Context, EventDimensionRow) error
}

type Provider interface {
	Descriptor() api.ImportProviderDescriptor
	Validate(context.Context, SourceSet) (*api.ImportManifest, error)
	Import(context.Context, SourceSet, Sink) (*api.ImportManifest, error)
}

type Registry struct {
	providers map[string]Provider
}

func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{providers: make(map[string]Provider, len(providers))}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		r.providers[provider.Descriptor().Key] = provider
	}
	return r
}

func (r *Registry) Provider(key string) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.providers[key]
	return p, ok
}

func (r *Registry) Descriptors() []api.ImportProviderDescriptor {
	if r == nil {
		return []api.ImportProviderDescriptor{}
	}
	descriptors := make([]api.ImportProviderDescriptor, 0, len(r.providers))
	for _, provider := range r.providers {
		descriptors = append(descriptors, provider.Descriptor())
	}
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].Name < descriptors[j].Name
	})
	return descriptors
}
