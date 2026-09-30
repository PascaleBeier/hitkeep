// Package analyticstools defines HitKeep's read-only AI tools once and adapts
// them to every surface that offers them: Ask AI and Opportunities through
// GoAI, and external assistants through MCP.
package analyticstools

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	goaisdk "github.com/zendev-sh/goai"

	"hitkeep/api"
	"hitkeep/database"
	json "hitkeep/jsonapi"
)

// Tool is one read-only, aggregate-only tool.
type Tool struct {
	Name        string
	Title       string
	Description string

	goai func(Scope) goaisdk.Tool
	mcp  func(*mcp.Server, Scope)
}

// Scope decides which site a call reads and which range it may read.
type Scope struct {
	// SiteID binds every call to one site and hides site_id from the model.
	// Leave it zero to let each call name a site, which Resolve authorizes.
	SiteID uuid.UUID
	// Resolve authorizes the caller for a site and returns its stores.
	Resolve func(ctx context.Context, siteID uuid.UUID) (Site, error)
	// From and To are the default range when a call names none.
	From, To time.Time
	// LockRange pins every call to From and To and hides range inputs.
	LockRange    bool
	MaxRangeDays int
	// Filters apply to every filterable call, ahead of the call's own.
	Filters []api.Filter
}

// Site is an authorized site and the stores that hold its data.
type Site struct {
	ID     uuid.UUID
	UserID uuid.UUID
	// Control holds team state: annotations, QR codes, integrations, and
	// Opportunities. Analytics holds hits.
	Control   *database.Store
	Analytics *database.Store
}

// Call is one authorized call against one site and range.
type Call struct {
	Site
	From, To     time.Time
	Filters      []api.Filter
	RangeLocked  bool
	MaxRangeDays int
}

// Target is embedded by every site-scoped tool input.
type Target struct {
	SiteID string `json:"site_id" jsonschema:"HitKeep site UUID."`
	From   string `json:"from,omitempty" jsonschema:"Optional RFC3339 start timestamp. Defaults to the current range, or 30 days before to."`
	To     string `json:"to,omitempty" jsonschema:"Optional RFC3339 end timestamp. Defaults to the current range, or now."`
}

func (t Target) target() Target { return t }

type targeted interface{ target() Target }

type filtered interface{ filterInputs() []FilterInput }

var (
	readOnly          = &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(false)}
	readOnlyOpenWorld = &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(true)}
)

// Define builds a Tool. Inputs that embed Target read one authorized site.
func Define[In, Out any](name, title, description string, run func(context.Context, Call, In) (Out, error)) Tool {
	return define(name, title, description, false, run)
}

// define also takes openWorld, which marks tools that fetch outside HitKeep.
func define[In, Out any](name, title, description string, openWorld bool, run func(context.Context, Call, In) (Out, error)) Tool {
	return Tool{
		Name:        name,
		Title:       title,
		Description: description,
		goai: func(scope Scope) goaisdk.Tool {
			return goaisdk.Tool{
				Name:        name,
				Description: description,
				InputSchema: goaiSchema[In](scope),
				Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
					var in In
					if len(raw) > 0 {
						if err := json.Unmarshal(raw, &in); err != nil {
							return "", errors.New("invalid tool input")
						}
					}
					call, err := scope.call(ctx, in)
					if err != nil {
						return "", err
					}
					out, err := run(ctx, call, in)
					if err != nil {
						return "", err
					}
					// The evidence ID is the tool name, so Ask AI and Opportunities
					// can check citations against the tools that actually ran.
					encoded, err := json.Marshal(map[string]any{"evidence_id": name, "data": out})
					if err != nil {
						return "", errors.New("encode tool result")
					}
					return string(encoded), nil
				},
			}
		},
		mcp: func(server *mcp.Server, scope Scope) {
			annotations := readOnly
			if openWorld {
				annotations = readOnlyOpenWorld
			}
			mcp.AddTool(server, &mcp.Tool{Name: name, Title: title, Description: description, Annotations: annotations},
				func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
					var zero Out
					call, err := scope.call(ctx, in)
					if err != nil {
						return nil, zero, err
					}
					out, err := run(ctx, call, in)
					return nil, out, err
				})
		},
	}
}

// GoAI adapts tools for a GoAI tool loop under one scope.
func GoAI(scope Scope, tools ...Tool) []goaisdk.Tool {
	out := make([]goaisdk.Tool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.goai(scope))
	}
	return out
}

// RegisterMCP adds tools to an MCP server under one scope.
func RegisterMCP(server *mcp.Server, scope Scope, tools ...Tool) {
	for _, tool := range tools {
		tool.mcp(server, scope)
	}
}

func (s Scope) call(ctx context.Context, in any) (Call, error) {
	t, ok := in.(targeted)
	if !ok {
		return Call{MaxRangeDays: s.MaxRangeDays}, nil
	}
	target := t.target()
	siteID := s.SiteID
	if siteID == uuid.Nil {
		parsed, err := uuid.Parse(strings.TrimSpace(target.SiteID))
		if err != nil {
			return Call{}, errors.New("invalid site_id")
		}
		siteID = parsed
	}
	from, to, err := s.window(target)
	if err != nil {
		return Call{}, err
	}
	var inputs []FilterInput
	if f, ok := in.(filtered); ok {
		inputs = f.filterInputs()
	}
	filters, err := parseFilters(inputs)
	if err != nil {
		return Call{}, err
	}
	if s.Resolve == nil {
		return Call{}, errors.New("analytics store unavailable")
	}
	site, err := s.Resolve(ctx, siteID)
	if err != nil {
		return Call{}, err
	}
	return Call{
		Site:         site,
		From:         from,
		To:           to,
		Filters:      append(slices.Clone(s.Filters), filters...),
		RangeLocked:  s.LockRange,
		MaxRangeDays: s.MaxRangeDays,
	}, nil
}

func (s Scope) window(target Target) (time.Time, time.Time, error) {
	named := strings.TrimSpace(target.From) != "" || strings.TrimSpace(target.To) != ""
	if !s.From.IsZero() && (s.LockRange || !named) {
		return s.From, s.To, nil
	}
	return parseRange(target.From, target.To, s.MaxRangeDays)
}

// A bound scope decides these inputs instead of the model.
var (
	siteInputs  = []string{"site_id"}
	rangeInputs = []string{"from", "to", "compare_from", "compare_to"}
)

func goaiSchema[In any](scope Scope) json.RawMessage {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		panic("analyticstools: input schema: " + err.Error())
	}
	if scope.SiteID != uuid.Nil {
		dropInputs(schema, siteInputs)
	}
	if scope.LockRange {
		dropInputs(schema, rangeInputs)
	}
	portable(schema)
	raw, err := json.Marshal(schema)
	if err != nil {
		panic("analyticstools: encode input schema: " + err.Error())
	}
	return raw
}

// portable drops "null" from type unions such as a Go slice's
// ["null","array"]: some providers, Gemini among them, reject union types in
// tool schemas. Omitting an optional input means the same as null.
func portable(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	if types := slices.DeleteFunc(slices.Clone(schema.Types), func(t string) bool { return t == "null" }); len(schema.Types) > 1 && len(types) == 1 {
		schema.Type, schema.Types = types[0], nil
	}
	portable(schema.Items)
	for _, property := range schema.Properties {
		portable(property)
	}
}

func dropInputs(schema *jsonschema.Schema, names []string) {
	for _, name := range names {
		delete(schema.Properties, name)
	}
	schema.Required = slices.DeleteFunc(schema.Required, func(name string) bool { return slices.Contains(names, name) })
}
