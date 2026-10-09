package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/ayush00git/aahat/api/internal/alert"
)

// DynamoStore keeps subscriptions and events in DynamoDB. Both tables only
// need a string partition key "pk"; items are prefixed ("SUB#", "EVT#") and
// carry a "type" attribute, so both can share one table.
//
// Each item holds the record as JSON in "doc", plus the attributes we filter
// on (place_osm, lake_id, created_at). Lists use filtered scans, which is
// fine at hackathon scale; a GSI on place_osm / lake_id is the next step.
type DynamoStore struct {
	db          *dynamodb.Client
	subsTable   string
	eventsTable string
}

func NewDynamoStore(db *dynamodb.Client, subsTable, eventsTable string) *DynamoStore {
	return &DynamoStore{db: db, subsTable: subsTable, eventsTable: eventsTable}
}

const (
	typeSubscription = "subscription"
	typeEvent        = "event"
)

func str(s string) types.AttributeValue { return &types.AttributeValueMemberS{Value: s} }

func (d *DynamoStore) CreateSubscription(ctx context.Context, s alert.Subscription) error {
	doc, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = d.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(d.subsTable),
		Item: map[string]types.AttributeValue{
			"pk":         str("SUB#" + s.ID),
			"type":       str(typeSubscription),
			"place_osm":  str(s.PlaceOSM),
			"created_at": str(s.CreatedAt.UTC().Format(time.RFC3339Nano)),
			"doc":        str(string(doc)),
		},
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	})
	return err
}

func (d *DynamoStore) DeleteSubscription(ctx context.Context, id string) error {
	_, err := d.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(d.subsTable),
		Key:                 map[string]types.AttributeValue{"pk": str("SUB#" + id)},
		ConditionExpression: aws.String("attribute_exists(pk)"),
	})
	var ccf *types.ConditionalCheckFailedException
	if errors.As(err, &ccf) {
		return ErrNotFound
	}
	return err
}

func (d *DynamoStore) ListSubscriptions(ctx context.Context, placeOSM string) ([]alert.Subscription, error) {
	filter := "#t = :t"
	values := map[string]types.AttributeValue{":t": str(typeSubscription)}
	if placeOSM != "" {
		filter += " AND place_osm = :p"
		values[":p"] = str(placeOSM)
	}
	subs, err := scanDocs[alert.Subscription](ctx, d.db, d.subsTable, filter, values)
	sortSubs(subs)
	return subs, err
}

func (d *DynamoStore) SubscriptionsForPlaces(ctx context.Context, osms []string) ([]alert.Subscription, error) {
	if len(osms) == 0 {
		return []alert.Subscription{}, nil
	}
	all, err := d.ListSubscriptions(ctx, "")
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(s alert.Subscription) bool {
		return !slices.Contains(osms, s.PlaceOSM)
	}), nil
}

func (d *DynamoStore) PutEvent(ctx context.Context, ev alert.Event) error {
	doc, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = d.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(d.eventsTable),
		Item: map[string]types.AttributeValue{
			"pk":         str("EVT#" + ev.ID),
			"type":       str(typeEvent),
			"lake_id":    str(ev.LakeID),
			"created_at": str(ev.CreatedAt.UTC().Format(time.RFC3339Nano)),
			"doc":        str(string(doc)),
		},
	})
	return err
}

func (d *DynamoStore) GetEvent(ctx context.Context, id string) (alert.Event, error) {
	out, err := d.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.eventsTable),
		Key:       map[string]types.AttributeValue{"pk": str("EVT#" + id)},
	})
	if err != nil {
		return alert.Event{}, err
	}
	if out.Item == nil {
		return alert.Event{}, ErrNotFound
	}
	var ev alert.Event
	err = decodeDoc(out.Item, &ev)
	return ev, err
}

func (d *DynamoStore) ListEvents(ctx context.Context, lakeID string) ([]alert.Event, error) {
	filter := "#t = :t"
	values := map[string]types.AttributeValue{":t": str(typeEvent)}
	if lakeID != "" {
		filter += " AND lake_id = :l"
		values[":l"] = str(lakeID)
	}
	evs, err := scanDocs[alert.Event](ctx, d.db, d.eventsTable, filter, values)
	sortEvents(evs)
	return evs, err
}

func scanDocs[T any](ctx context.Context, db *dynamodb.Client, table, filter string, values map[string]types.AttributeValue) ([]T, error) {
	p := dynamodb.NewScanPaginator(db, &dynamodb.ScanInput{
		TableName:                 aws.String(table),
		FilterExpression:          aws.String(filter),
		ExpressionAttributeNames:  map[string]string{"#t": "type"}, // "type" is a reserved word
		ExpressionAttributeValues: values,
		ProjectionExpression:      aws.String("doc"),
	})
	out := []T{}
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			var v T
			if err := decodeDoc(item, &v); err != nil {
				return nil, err
			}
			out = append(out, v)
		}
	}
	return out, nil
}

func decodeDoc(item map[string]types.AttributeValue, v any) error {
	doc, ok := item["doc"].(*types.AttributeValueMemberS)
	if !ok {
		return fmt.Errorf("item has no doc attribute")
	}
	return json.Unmarshal([]byte(doc.Value), v)
}
