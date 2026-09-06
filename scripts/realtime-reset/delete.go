package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"go-boilerplate/pkg/xerrors"
)

// deleteTables は、渡された table を順に削除し、消え切るまで待ちます。
// 既に無い table は、この関数の目的（無い状態にすること）から見れば成功なので読み飛ばします。
func deleteTables(ctx context.Context, api tableAPI, tables []string, out io.Writer) error {
	for _, table := range tables {
		deleted, err := deleteTable(ctx, api, table)
		if err != nil {
			return xerrors.Wrap(err, "realtime-reset: "+table)
		}

		if !deleted {
			_, _ = fmt.Fprintf(out, "・%s は既にありません\n", table)

			continue
		}

		if err := waitGone(ctx, api, table); err != nil {
			return xerrors.Wrap(err, "realtime-reset: "+table)
		}

		_, _ = fmt.Fprintf(out, "・%s を削除しました\n", table)
	}

	return nil
}

// deleteTable は table を削除し、削除を要求したかどうかを返します。
func deleteTable(ctx context.Context, api tableAPI, table string) (bool, error) {
	_, err := api.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(table)})
	if err == nil {
		return true, nil
	}

	var notFound *dynamodbtypes.ResourceNotFoundException
	if xerrors.As(err, &notFound) {
		return false, nil
	}

	return false, xerrors.Wrap(err, "delete table")
}

// waitGone は、table が引けなくなるまで待ちます。DeleteTable は非同期に返るため、待たずに
// realtime-init へ進むと作り直しが ResourceInUseException で落ちます。
func waitGone(ctx context.Context, api tableAPI, table string) error {
	for {
		_, err := api.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(table)})

		var notFound *dynamodbtypes.ResourceNotFoundException
		if xerrors.As(err, &notFound) {
			return nil
		}

		if err != nil {
			return xerrors.Wrap(err, "describe table")
		}

		select {
		case <-ctx.Done():
			return xerrors.Join(errGone, xerrors.Wrap(ctx.Err(), table))
		case <-time.After(gonePollInterval):
		}
	}
}
