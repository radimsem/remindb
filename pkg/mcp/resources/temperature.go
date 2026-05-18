package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/store"
)

const TemperatureURI = "remindb://temperature"

const hotThreshold = 0.5

type tempSummary struct {
	Avg           float64 `json:"avg"`
	Median        float64 `json:"median"`
	Hot           int     `json:"hot"`
	Cold          int     `json:"cold"`
	Pinned        int     `json:"pinned"`
	ColdThreshold float64 `json:"cold_threshold"`
	HotThreshold  float64 `json:"hot_threshold"`
}

type tempNode struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	Temperature float64 `json:"temperature"`
	Pinned      bool    `json:"pinned"`
}

type temperatureResourceEnvelope struct {
	Summary tempSummary `json:"summary"`
	Nodes   []tempNode  `json:"nodes"`
}

func newTemperatureEnvelope(all []*store.Node, coldThreshold float64) temperatureResourceEnvelope {
	env := temperatureResourceEnvelope{
		Summary: tempSummary{ColdThreshold: coldThreshold, HotThreshold: hotThreshold},
		Nodes:   make([]tempNode, 0, len(all)),
	}
	if len(all) == 0 {
		return env
	}

	temps := make([]float64, 0, len(all))
	var sum float64
	for _, n := range all {
		env.Nodes = append(env.Nodes, tempNode{
			ID:          n.ID,
			Label:       n.Label,
			Temperature: n.Temperature,
			Pinned:      n.Pinned,
		})

		temps = append(temps, n.Temperature)
		sum += n.Temperature
		if n.Temperature >= hotThreshold {
			env.Summary.Hot++
		}
		if n.Temperature < coldThreshold {
			env.Summary.Cold++
		}
		if n.Pinned {
			env.Summary.Pinned++
		}
	}

	sort.Float64s(temps)
	env.Summary.Avg = sum / float64(len(all))
	env.Summary.Median = temps[len(temps)/2]

	return env
}

func (d *Deps) HandleTemperature(ctx context.Context, _ *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	all, err := d.Store.GetAllNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get: temperature nodes: %w", err)
	}

	body, err := json.Marshal(newTemperatureEnvelope(all, d.ColdThreshold))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: temperature: %w", err)
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      TemperatureURI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}
