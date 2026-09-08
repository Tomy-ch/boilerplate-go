package useridentity

import (
	"context"

	"go-boilerplate/internal/infrastructure/rdb/driver"
	"go-boilerplate/internal/infrastructure/rdb/pgerror"
	"go-boilerplate/internal/infrastructure/rdb/sqlc/gen"
	"go-boilerplate/internal/observability"
	authbd "go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

type registrar struct {
	db     driver.DatabaseDriver
	tracer observability.LayerTracer
}

// NewRegistrar は、authbd.IdentityRegistrar の RDB 実装を生成して返します。
func NewRegistrar(
	db driver.DatabaseDriver,
	tf observability.TracerFactory,
) authbd.IdentityRegistrar {
	return &registrar{
		db:     db,
		tracer: tf.Infra(),
	}
}

// Register は、issuer と subject の組を 1 行として書きます。同じ組が既にある場合は
// 一意制約違反が、在籍しない利用者への結び付けは外部キー違反が、それぞれ正規化されて返ります。
func (r *registrar) Register(ctx context.Context, userID uuid.UUID, issuer, subject string) error {
	ctx, endSpan := r.tracer.Start(ctx)
	defer endSpan()

	id, err := uuid.New()
	if err != nil {
		return xerrors.Wrap(err, "failed to generate user identity id")
	}

	db := gen.New(driver.New(ctx, r.db))
	if err = db.CreateUserIdentity(ctx, &gen.CreateUserIdentityParams{
		ID:      id,
		UserID:  userID,
		Issuer:  issuer,
		Subject: subject,
	}); err != nil {
		return pgerror.NormalizeError(err)
	}

	return nil
}
