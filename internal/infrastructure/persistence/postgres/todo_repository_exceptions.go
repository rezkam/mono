package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rezkam/mono/internal/domain"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres/sqlcgen"
)

// isUniqueViolation checks if an error is a PostgreSQL unique constraint violation
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgerrcode.UniqueViolation
	}
	return false
}

func (s *Store) CreateException(ctx context.Context, exception *domain.RecurringTemplateException) (*domain.RecurringTemplateException, error) {
	idUUID, err := uuid.Parse(exception.ID)
	if err != nil {
		return nil, err
	}
	templateUUID, err := uuid.Parse(exception.TemplateID)
	if err != nil {
		return nil, err
	}

	var itemUUID pgtype.UUID
	if exception.ItemID != nil {
		parsedItemUUID, err := uuid.Parse(*exception.ItemID)
		if err != nil {
			return nil, err
		}
		itemUUID = pgtype.UUID{Bytes: parsedItemUUID, Valid: true}
	}

	dbException, err := s.queries.CreateException(ctx, sqlcgen.CreateExceptionParams{
		ID:            pgtype.UUID{Bytes: idUUID, Valid: true},
		TemplateID:    pgtype.UUID{Bytes: templateUUID, Valid: true},
		OccursAt:      timeToTimestamptz(exception.OccursAt),
		ExceptionType: string(exception.ExceptionType),
		ItemID:        itemUUID,
		CreatedAt:     timeToTimestamptz(exception.CreatedAt),
	})

	if err != nil {
		// Check for unique constraint violation
		if isUniqueViolation(err) {
			return nil, domain.ErrExceptionAlreadyExists
		}
		return nil, err
	}

	return dbExceptionToDomain(dbException)
}

func (s *Store) FindExceptions(ctx context.Context, templateID string, from, until time.Time) ([]*domain.RecurringTemplateException, error) {
	templateUUID, err := uuid.Parse(templateID)
	if err != nil {
		return nil, err
	}

	dbExceptions, err := s.queries.FindExceptions(ctx, sqlcgen.FindExceptionsParams{
		TemplateID: pgtype.UUID{Bytes: templateUUID, Valid: true},
		OccursAt:   timeToTimestamptz(from),
		OccursAt_2: timeToTimestamptz(until),
	})

	if err != nil {
		return nil, err
	}

	exceptions := make([]*domain.RecurringTemplateException, len(dbExceptions))
	for i, dbExc := range dbExceptions {
		exc, err := dbExceptionToDomain(dbExc)
		if err != nil {
			return nil, err
		}
		exceptions[i] = exc
	}

	return exceptions, nil
}

func (s *Store) FindExceptionByOccurrence(ctx context.Context, templateID string, occursAt time.Time) (*domain.RecurringTemplateException, error) {
	templateUUID, err := uuid.Parse(templateID)
	if err != nil {
		return nil, err
	}

	dbException, err := s.queries.FindExceptionByOccurrence(ctx, sqlcgen.FindExceptionByOccurrenceParams{
		TemplateID: pgtype.UUID{Bytes: templateUUID, Valid: true},
		OccursAt:   timeToTimestamptz(occursAt),
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrExceptionNotFound
		}
		return nil, err
	}

	return dbExceptionToDomain(dbException)
}

func (s *Store) DeleteException(ctx context.Context, templateID string, occursAt time.Time) error {
	templateUUID, err := uuid.Parse(templateID)
	if err != nil {
		return err
	}

	err = s.queries.DeleteException(ctx, sqlcgen.DeleteExceptionParams{
		TemplateID: pgtype.UUID{Bytes: templateUUID, Valid: true},
		OccursAt:   timeToTimestamptz(occursAt),
	})

	if err != nil {
		return err
	}

	return nil
}

func (s *Store) ListAllExceptionsByTemplate(ctx context.Context, templateID string) ([]*domain.RecurringTemplateException, error) {
	templateUUID, err := uuid.Parse(templateID)
	if err != nil {
		return nil, err
	}

	dbExceptions, err := s.queries.ListAllExceptionsByTemplate(ctx, pgtype.UUID{Bytes: templateUUID, Valid: true})

	if err != nil {
		return nil, err
	}

	exceptions := make([]*domain.RecurringTemplateException, len(dbExceptions))
	for i, dbExc := range dbExceptions {
		exc, err := dbExceptionToDomain(dbExc)
		if err != nil {
			return nil, err
		}
		exceptions[i] = exc
	}

	return exceptions, nil
}
