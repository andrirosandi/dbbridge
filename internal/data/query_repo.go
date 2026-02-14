package data

import (
	"database/sql"
	"dbbridge/internal/core"
)

type QueryRepo struct {
	db *sql.DB
}

func NewQueryRepo(db *sql.DB) *QueryRepo {
	return &QueryRepo{db: db}
}

func (r *QueryRepo) Create(q *core.SavedQuery) error {
	res, err := r.db.Exec(`INSERT INTO queries (slug, description, sql_text, params_config, is_active) VALUES (?, ?, ?, ?, ?)`,
		q.Slug, q.Description, q.SQLText, q.ParamsConfig, q.IsActive)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	q.ID = id

	return r.updateLinks(q.ID, q.AllowedConnectionIDs)
}

func (r *QueryRepo) GetByID(id int64) (*core.SavedQuery, error) {
	var q core.SavedQuery
	var isActive int
	err := r.db.QueryRow(`SELECT id, slug, description, sql_text, params_config, is_active FROM queries WHERE id = ?`, id).
		Scan(&q.ID, &q.Slug, &q.Description, &q.SQLText, &q.ParamsConfig, &isActive)
	if err != nil {
		return nil, err
	}
	q.IsActive = isActive == 1

	q.AllowedConnectionIDs, err = r.getLinks(q.ID)
	if err != nil {
		return nil, err
	}

	return &q, nil
}

func (r *QueryRepo) GetBySlug(slug string) (*core.SavedQuery, error) {
	var q core.SavedQuery
	var isActive int
	err := r.db.QueryRow(`SELECT id, slug, description, sql_text, params_config, is_active FROM queries WHERE slug = ?`, slug).
		Scan(&q.ID, &q.Slug, &q.Description, &q.SQLText, &q.ParamsConfig, &isActive)
	if err != nil {
		return nil, err
	}
	q.IsActive = isActive == 1

	q.AllowedConnectionIDs, err = r.getLinks(q.ID)
	if err != nil {
		return nil, err
	}

	return &q, nil
}

func (r *QueryRepo) GetAll() ([]core.SavedQuery, error) {
	rows, err := r.db.Query(`SELECT id, slug, description, sql_text, params_config, is_active FROM queries`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queries []core.SavedQuery
	for rows.Next() {
		var q core.SavedQuery
		var isActive int
		if err := rows.Scan(&q.ID, &q.Slug, &q.Description, &q.SQLText, &q.ParamsConfig, &isActive); err != nil {
			return nil, err
		}
		q.IsActive = isActive == 1

		// Optimization: fetch links in loop (N+1) but fine for small scale.
		// Better: Fetch all links and map. For now keep simple.
		q.AllowedConnectionIDs, _ = r.getLinks(q.ID)

		queries = append(queries, q)
	}
	return queries, nil
}

func (r *QueryRepo) Update(q *core.SavedQuery) error {
	_, err := r.db.Exec(`UPDATE queries SET slug=?, description=?, sql_text=?, params_config=?, is_active=? WHERE id=?`,
		q.Slug, q.Description, q.SQLText, q.ParamsConfig, q.IsActive, q.ID)
	if err != nil {
		return err
	}
	return r.updateLinks(q.ID, q.AllowedConnectionIDs)
}

func (r *QueryRepo) Delete(id int64) error {
	// Cascade delete should handle links, but let's be safe/explicit if needed.
	// SQLite FKs need enabling. Assuming they are enabled or we rely on them.
	// Actually `db.go` Create table used ON DELETE CASCADE.
	// Verify if PRAGMA foreign_keys = ON is set? It's not default in SQLite.
	// Let's manually delete links first to be sure.
	r.db.Exec(`DELETE FROM query_connections WHERE query_id=?`, id)
	_, err := r.db.Exec(`DELETE FROM queries WHERE id=?`, id)
	return err
}

// Helper methods for links
func (r *QueryRepo) updateLinks(queryID int64, connIDs []int64) error {
	// Transaction?
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	// 1. Delete existing
	_, err = tx.Exec(`DELETE FROM query_connections WHERE query_id = ?`, queryID)
	if err != nil {
		tx.Rollback()
		return err
	}

	// 2. Insert new
	stmt, err := tx.Prepare(`INSERT INTO query_connections (query_id, connection_id) VALUES (?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, connID := range connIDs {
		_, err = stmt.Exec(queryID, connID)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (r *QueryRepo) getLinks(queryID int64) ([]int64, error) {
	rows, err := r.db.Query(`SELECT connection_id FROM query_connections WHERE query_id = ?`, queryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
