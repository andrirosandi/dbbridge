package data

import (
	"database/sql"
	"dbbridge/internal/core"
)

type ConnectionRepo struct {
	db *sql.DB
}

func NewConnectionRepo(db *sql.DB) *ConnectionRepo {
	return &ConnectionRepo{db: db}
}

func (r *ConnectionRepo) Create(conn *core.DBConnection) error {
	query := `INSERT INTO connections (name, driver, connection_string_enc, is_active) VALUES (?, ?, ?, ?)`
	res, err := r.db.Exec(query, conn.Name, conn.Driver, conn.ConnectionStringEnc, conn.IsActive)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	conn.ID = id
	return nil
}

func (r *ConnectionRepo) GetAll() ([]core.DBConnection, error) {
	rows, err := r.db.Query(`SELECT id, name, driver, connection_string_enc, is_active FROM connections`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var connections []core.DBConnection
	for rows.Next() {
		var c core.DBConnection
		// SQLite stores booleans as integers (0 or 1)
		var isActive int
		if err := rows.Scan(&c.ID, &c.Name, &c.Driver, &c.ConnectionStringEnc, &isActive); err != nil {
			return nil, err
		}
		c.IsActive = isActive == 1
		connections = append(connections, c)
	}
	return connections, nil
}

func (r *ConnectionRepo) GetByID(id int64) (*core.DBConnection, error) {
	var c core.DBConnection
	var isActive int
	err := r.db.QueryRow(`SELECT id, name, driver, connection_string_enc, is_active FROM connections WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.Driver, &c.ConnectionStringEnc, &isActive)
	if err != nil {
		return nil, err
	}
	c.IsActive = isActive == 1
	return &c, nil
}

func (r *ConnectionRepo) Update(conn *core.DBConnection) error {
	_, err := r.db.Exec(`UPDATE connections SET name=?, driver=?, connection_string_enc=?, is_active=? WHERE id=?`,
		conn.Name, conn.Driver, conn.ConnectionStringEnc, conn.IsActive, conn.ID)
	return err
}

func (r *ConnectionRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM connections WHERE id=?`, id)
	return err
}
