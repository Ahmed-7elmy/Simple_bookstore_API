package books

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("book not found")

type Book struct {
	ID              int64     `json:"id"`
	Title           string    `json:"title"`
	Author          string    `json:"author"`
	PublicationYear int       `json:"publication_year"`
	CreatedAt       time.Time `json:"created_at"`
}

type Store interface {
	List(context.Context) ([]Book, error)
	Get(context.Context, int64) (Book, error)
	Create(context.Context, Book) (Book, error)
	Update(context.Context, int64, Book) (Book, error)
	Delete(context.Context, int64) error
}

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) List(ctx context.Context) ([]Book, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, author, publication_year, created_at
		FROM books
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}
	defer rows.Close()

	result := make([]Book, 0)
	for rows.Next() {
		book, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, book)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read books: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) Get(ctx context.Context, id int64) (Book, error) {
	book, err := scanBook(s.db.QueryRowContext(ctx, `
		SELECT id, title, author, publication_year, created_at
		FROM books
		WHERE id = $1
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Book{}, ErrNotFound
	}
	if err != nil {
		return Book{}, fmt.Errorf("get book: %w", err)
	}
	return book, nil
}

func (s *PostgresStore) Create(ctx context.Context, book Book) (Book, error) {
	result, err := scanBook(s.db.QueryRowContext(ctx, `
		INSERT INTO books (title, author, publication_year)
		VALUES ($1, $2, $3)
		RETURNING id, title, author, publication_year, created_at
	`, book.Title, book.Author, book.PublicationYear))
	if err != nil {
		return Book{}, fmt.Errorf("create book: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) Update(ctx context.Context, id int64, book Book) (Book, error) {
	result, err := scanBook(s.db.QueryRowContext(ctx, `
		UPDATE books
		SET title = $1, author = $2, publication_year = $3
		WHERE id = $4
		RETURNING id, title, author, publication_year, created_at
	`, book.Title, book.Author, book.PublicationYear, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Book{}, ErrNotFound
	}
	if err != nil {
		return Book{}, fmt.Errorf("update book: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) Delete(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM books WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete book: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check deleted book: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

type bookScanner interface {
	Scan(dest ...any) error
}

func scanBook(row bookScanner) (Book, error) {
	var book Book
	if err := row.Scan(&book.ID, &book.Title, &book.Author, &book.PublicationYear, &book.CreatedAt); err != nil {
		return Book{}, err
	}
	return book, nil
}
