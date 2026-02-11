package controller

// issueLabel represents a GitHub issue label.
type issueLabel struct {
	Name string `json:"name"`
}

// issueCommentAuthor represents the author of a GitHub issue comment.
type issueCommentAuthor struct {
	Login string `json:"login"`
}

// issueComment represents a single comment on a GitHub issue.
type issueComment struct {
	Author    issueCommentAuthor `json:"author"`
	Body      string             `json:"body"`
	CreatedAt string             `json:"createdAt"`
}

type issueDetail struct {
	Number    int            `json:"number"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Labels    []issueLabel   `json:"labels"`
	Comments  []issueComment `json:"comments"`
	DependsOn []string       // Parsed dependency issue IDs (populated by buildDependencyGraph)
}
