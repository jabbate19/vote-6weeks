package database

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Poll struct {
	Id               string   `bson:"_id,omitempty"`
	CreatedBy        string   `bson:"createdBy"`
	ShortDescription string   `bson:"shortDescription"`
	LongDescription  string   `bson:"longDescription"`
	VoteType         string   `bson:"voteType"`
	Options          []string `bson:"options"`
	Open             bool     `bson:"open"`
	Hidden           bool     `bson:"hidden"`
	AllowWriteIns    bool     `bson:"writeins"`
}

type Result struct {
	Option string `bson:"_id"`
	Count  int    `bson:"count"`
}

const POLL_TYPE_SIMPLE = "simple"
const POLL_TYPE_RANKED = "ranked"

func GetPoll(id string) (*Poll, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	objId, _ := primitive.ObjectIDFromHex(id)
	var poll Poll
	if err := Client.Database("vote").Collection("polls").FindOne(ctx, map[string]interface{}{"_id": objId}).Decode(&poll); err != nil {
		return nil, err
	}

	return &poll, nil
}

func (poll *Poll) Close() error {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	objId, _ := primitive.ObjectIDFromHex(poll.Id)

	_, err := Client.Database("vote").Collection("polls").UpdateOne(ctx, map[string]interface{}{"_id": objId}, map[string]interface{}{"$set": map[string]interface{}{"open": false}})
	if err != nil {
		return err
	}

	return nil
}

func CreatePoll(poll *Poll) (string, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	result, err := Client.Database("vote").Collection("polls").InsertOne(ctx, poll)
	if err != nil {
		return "", err
	}

	return result.InsertedID.(primitive.ObjectID).Hex(), nil
}

func GetOpenPolls() ([]*Poll, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	cursor, err := Client.Database("vote").Collection("polls").Find(ctx, map[string]interface{}{"open": true})
	if err != nil {
		return nil, err

	}

	var polls []*Poll
	cursor.All(ctx, &polls)

	return polls, nil
}

func GetClosedOwnedPolls(userId string) ([]*Poll, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	cursor, err := Client.Database("vote").Collection("polls").Find(ctx, map[string]interface{}{"createdBy": userId, "open": false})
	if err != nil {
		return nil, err
	}

	var polls []*Poll
	cursor.All(ctx, &polls)

	return polls, nil
}

func GetClosedVotedPolls(userId string) ([]*Poll, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	cursor, err := Client.Database("vote").Collection("votes").Aggregate(ctx, mongo.Pipeline{
		{{
			"$match", bson.D{
				{"userId", userId},
			},
		}},
		{{
			"$lookup", bson.D{
				{"from", "polls"},
				{"localField", "pollId"},
				{"foreignField", "_id"},
				{"as", "polls"},
			},
		}},
		{{
			"$unwind", bson.D{
				{"path", "$polls"},
				{"preserveNullAndEmptyArrays", false},
			},
		}},
		{{
			"$replaceRoot", bson.D{
				{"newRoot", "$polls"},
			},
		}},
		{{
			"$match", bson.D{
				{"open", false},
			},
		}},
	})
	if err != nil {
		return nil, err
	}

	var polls []*Poll
	cursor.All(ctx, &polls)

	return polls, nil
}

func (poll *Poll) GetResult() (map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	pollId, _ := primitive.ObjectIDFromHex(poll.Id)

	cursor, err := Client.Database("vote").Collection("votes").Aggregate(ctx, mongo.Pipeline{
		{{
			"$match", bson.D{
				{"pollId", pollId},
			},
		}},
		{{
			"$group", bson.D{
				{"_id", "$option"},
				{"count", bson.D{
					{"$sum", 1},
				}},
			},
		}},
	})
	if err != nil {
		return nil, err
	}

	var results []Result
	cursor.All(ctx, &results)

	result := make(map[string]int)
	for _, r := range results {
		result[r.Option] = r.Count
	}

	return result, nil
}
