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
	return nil
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
		queries = append(queries, q)
	}
	return queries, nil
}

func (r *QueryRepo) Update(q *core.SavedQuery) error {
	_, err := r.db.Exec(`UPDATE queries SET slug=?, description=?, sql_text=?, params_config=?, is_active=? WHERE id=?`,
		q.Slug, q.Description, q.SQLText, q.ParamsConfig, q.IsActive, q.ID)
	return err
}

func (r *QueryRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM queries WHERE id=?`, id)
	return err
}
