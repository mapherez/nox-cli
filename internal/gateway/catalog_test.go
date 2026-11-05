package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	noxmcp "github.com/mapherez/nox-mcp/go"
)

func catalogFixture(t *testing.T, count int) (*noxmcp.DiscoveryService, noxmcp.DiscoveryView) {
	t.Helper()
	service, err := noxmcp.NewDiscovery("gateway-test")
	if err != nil {
		t.Fatal(err)
	}
	view := noxmcp.DiscoveryView{ViewID: "authorized", Presentation: noxmcp.JSON{"products": []any{noxmcp.JSON{"productId": "nox-backend", "displayName": "NoX Backend"}}}}
	for i := 0; i < count; i++ {
		view.Definitions = append(view.Definitions, noxmcp.JSON{"tool": noxmcp.JSON{"name": fmt.Sprintf("alias-%03d", i), "inputSchema": noxmcp.JSON{"type": "object"}, "_meta": noxmcp.JSON{"cli": fmt.Sprintf("vault action%d", i), "nox/gateway": noxmcp.JSON{"version": "1.0", "productId": "nox-backend"}}}, "cli": noxmcp.JSON{"version": "1.0", "navigation": noxmcp.JSON{"group": "Vaults"}}, "availability": "unavailable"})
	}
	return service, view
}

// Normalize through JSON, exactly as the MCP client decodes wire responses.
func wireJSON(t *testing.T, v noxmcp.JSON) noxmcp.JSON {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var normalized noxmcp.JSON
	if err := json.Unmarshal(b, &normalized); err != nil {
		t.Fatal(err)
	}
	return normalized
}

func TestVerifiedCatalogPaginationDefinitionBatchesAndAvailability(t *testing.T) {
	service, view := catalogFixture(t, 130)
	pages, batches := 0, 0
	request := func(_ context.Context, method string, p noxmcp.JSON) (noxmcp.JSON, error) {
		var response noxmcp.JSON
		var err error
		if method == "nox/tools/catalog" {
			pages++
			if p["cursor"] == nil {
				p["limit"] = 2
			}
			response, err = service.Catalog(view, p)
		} else {
			batches++
			if len(p["tools"].([]any)) > 64 {
				t.Fatal("oversized batch")
			}
			response, err = service.Definitions(view, p)
		}
		if err != nil {
			return nil, err
		}
		return wireJSON(t, response), nil
	}
	catalog, err := readCatalog(context.Background(), request)
	if err != nil || len(catalog.Definitions) != 130 || pages != 2 || batches != 3 {
		t.Fatal(len(catalog.Definitions), pages, batches, err)
	}
	if catalog.Definitions["alias-000"]["availability"] != "unavailable" || len(catalog.Presentation) == 0 {
		t.Fatal("presentation or catalog availability lost")
	}
	if catalog.Fingerprint == "" || catalog.TTL == 0 {
		t.Fatal("snapshot identity missing")
	}
}

func TestCatalogRejectsChangedIdentityDefinitionsAndMissingPages(t *testing.T) {
	for _, fault := range []string{"catalog-hash", "definition-body", "missing-definition", "changed-page", "empty-page"} {
		t.Run(fault, func(t *testing.T) {
			service, view := catalogFixture(t, 3)
			request := func(_ context.Context, method string, p noxmcp.JSON) (noxmcp.JSON, error) {
				var response noxmcp.JSON
				var err error
				if method == "nox/tools/catalog" {
					if p["cursor"] == nil {
						p["limit"] = 1
					}
					response, err = service.Catalog(view, p)
				} else {
					response, err = service.Definitions(view, p)
				}
				if err != nil {
					return nil, err
				}
				response = wireJSON(t, response)
				if method == "nox/tools/catalog" {
					if fault == "catalog-hash" {
						response["catalog"] = "invalid"
					}
					if p["cursor"] != nil && fault == "changed-page" {
						response["viewId"] = "changed"
					}
					if p["cursor"] != nil && fault == "empty-page" {
						response["tools"] = []any{}
					}
				} else {
					definitions := response["definitions"].([]any)
					if fault == "definition-body" {
						definitions[0].(map[string]any)["tool"].(map[string]any)["description"] = "tampered"
					}
					if fault == "missing-definition" {
						response["definitions"] = definitions[:len(definitions)-1]
					}
				}
				return response, nil
			}
			if _, err := readCatalog(context.Background(), request); err == nil || (fault != "catalog-hash" && !errors.Is(err, ErrProtocol)) {
				t.Fatal("unverified snapshot accepted", err)
			}
		})
	}
}
