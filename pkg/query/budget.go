package query

type Result struct {
	Nodes      []ScoredNode
	TokensUsed int
}

func allNodes(scored []ScoredNode) Result {
	used := 0
	for _, sn := range scored {
		used += sn.Node.TokenCount
	}

	return Result{Nodes: scored, TokensUsed: used}
}

func fillBudget(scored []ScoredNode, budget int) Result {
	var out []ScoredNode
	used := 0

	for _, sn := range scored {
		cost := sn.Node.TokenCount
		if used+cost > budget {
			continue
		}
		out = append(out, sn)
		used += cost
	}
	return Result{Nodes: out, TokensUsed: used}
}
