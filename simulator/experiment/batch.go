package experiment

import "context"

type Request struct {
	Input   Input   `json:"input"`
	Variant Variant `json:"variant"`
	Seed    uint64  `json:"seed"`
}

func (g Generator) Generate(ctx context.Context, requests []Request) ([]Projection, error) {
	out := make([]Projection, 0, len(requests))
	for _, r := range requests {
		p, e := g.Predict(ctx, r.Input, r.Variant, r.Seed)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, nil
}
