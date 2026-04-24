package system

import "maps"

func op(tags []string, summary string, description string, security []any, parameters []any, requestBody any, responses map[string]any) map[string]any {
	out := map[string]any{
		"tags":        tags,
		"summary":     summary,
		"description": description,
		"responses":   responses,
	}
	if len(security) > 0 {
		out["security"] = security
	}
	if len(parameters) > 0 {
		out["parameters"] = parameters
	}
	if requestBody != nil {
		out["requestBody"] = requestBody
	}
	return out
}

func cloudOp(summary string, description string, security []any, parameters []any, requestBody any, responses map[string]any) map[string]any {
	out := op([]string{"Cloud"}, summary, description, security, parameters, requestBody, responses)
	out["x-hitkeep-availability"] = "cloud"
	out["x-hitkeep-build-tags"] = []string{"billing"}
	return out
}

func paramRef(ref string) map[string]any {
	return map[string]any{"$ref": ref}
}

func secCookie() []any {
	return []any{map[string]any{"cookieAuth": []any{}}}
}

func secAnyAuth() []any {
	return []any{
		map[string]any{"cookieAuth": []any{}},
		map[string]any{"bearerAuth": []any{}},
		map[string]any{"apiKeyAuth": []any{}},
	}
}

func jsonBody(schema map[string]any) map[string]any {
	return map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": schema,
			},
		},
	}
}

func desc(description string) map[string]any {
	return map[string]any{"description": description}
}

func errResp(description string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{"$ref": "#/components/schemas/Error"},
			},
		},
	}
}

func jsonRefResp(description string, ref string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{"$ref": ref},
			},
		},
	}
}

func jsonSchemaResp(description string, schema map[string]any) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": schema,
			},
		},
	}
}

func mergeOpenAPIPathMaps(groups ...map[string]any) map[string]any {
	paths := make(map[string]any)
	for _, group := range groups {
		maps.Copy(paths, group)
	}
	return paths
}

func mergeOpenAPIMapGroups(groups ...map[string]any) map[string]any {
	merged := make(map[string]any)
	for _, group := range groups {
		maps.Copy(merged, group)
	}
	return merged
}
