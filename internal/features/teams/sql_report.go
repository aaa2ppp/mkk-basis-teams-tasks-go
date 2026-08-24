package teams

import (
	"context"
	"fmt"
	"strconv"

	"aaa2ppp/teams-tasks/internal/model"
)

func (s *storage) GenReport(ctx context.Context, req DBGenReportReq) ([]Metric, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH access AS (
			SELECT 1
			FROM team_members
			WHERE team_id = ? AND user_id = ? AND role IN ('owner', 'admin')
		),
		status_stats AS (
			SELECT 'status_stats' AS metric_type, status AS detail, COUNT(*) AS value
			FROM tasks t
			CROSS JOIN access a
			WHERE t.team_id = ?
			GROUP BY status
		),
		top3_assignees AS (
			SELECT 'top3_assignees' AS metric_type, CONCAT(u.id, ':', u.name) AS detail, COUNT(t.id) AS value
			FROM tasks t
			CROSS JOIN access a
			LEFT JOIN users u ON u.id = t.assignee_id
			WHERE t.team_id = ? AND t.closed_at >= CURDATE() - INTERVAL 30 DAY
			GROUP BY u.id, u.name
			ORDER BY value DESC
			LIMIT 3
		),
		avg_close_days AS (
			SELECT 'avg_close_days' AS metric_type, 'all' AS detail, AVG(DATEDIFF(closed_at, created_at) + 1) AS value
			FROM tasks t
			CROSS JOIN access a
			WHERE t.team_id = ? AND t.closed_at IS NOT NULL
		),
		total_comments AS (
			SELECT 'total_comments' AS metric_type, '' AS detail, COUNT(tc.id) AS value
			FROM tasks t
			CROSS JOIN access a
			JOIN task_comments tc ON tc.task_id = t.id
			WHERE t.team_id = ?
		)
		SELECT * FROM status_stats
		UNION ALL
		SELECT * FROM top3_assignees
		UNION ALL
		SELECT metric_type, detail, value FROM avg_close_days CROSS JOIN access
		UNION ALL
		SELECT metric_type, detail, value FROM total_comments CROSS JOIN access;`,
		req.TeamID, req.CurUserID, // access
		req.TeamID, // status_stats
		req.TeamID, // top3_assignees
		req.TeamID, // avg_close_days
		req.TeamID, // total_comments
	)
	if err != nil {
		return nil, err
	}

	var metrics []Metric
	var r Metric
	for rows.Next() {
		r = Metric{}
		if err := rows.Scan(&r.Type, &r.Detail, &r.Value); err != nil {
			return nil, err
		}
		metrics = append(metrics, r)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(metrics) == 0 {
		return nil, model.ErrForbidden
	}

	fixStatus(metrics)
	return metrics, nil
}

func fixStatus(metrics []Metric) {
	for i := range metrics {
		if metrics[i].Type == "status_stats" {
			if v, err := strconv.Atoi(metrics[i].Detail); err != nil {
				metrics[i].Detail = fmt.Sprintf("<!ERROR: %v>", err)
			} else {
				metrics[i].Detail = model.Status(v).String()
			}
		}
	}
}
