package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TaskFinder struct {
	pool *pgxpool.Pool
}

func NewTaskFinder(pool *pgxpool.Pool) *TaskFinder {
	return &TaskFinder{pool: pool}
}

func (f *TaskFinder) FindTaskMetadata(ctx context.Context, taskID string) (string, string, string, error) {
	if f == nil || f.pool == nil {
		return "", "", "", nil
	}
	var compName, outcomeName, content string
	err := f.pool.QueryRow(ctx, `
		SELECT
			c.name,
			o.name,
			COALESCE(o.educational_content, '')
		FROM tasks t
		JOIN outcomes o ON o.id = t.outcome_id
		JOIN constituents const ON const.id = o.constituent_id
		JOIN competencies c ON c.id = const.competency_id
		WHERE t.id = $1
	`, taskID).Scan(&compName, &outcomeName, &content)
	return compName, outcomeName, content, err
}
