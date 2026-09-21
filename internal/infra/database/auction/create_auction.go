package auction

import (
	"context"
	"fullcycle-auction_go/configuration/logger"
	"fullcycle-auction_go/internal/entity/auction_entity"
	"fullcycle-auction_go/internal/internal_error"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type AuctionEntityMongo struct {
	Id          string                          `bson:"_id"`
	ProductName string                          `bson:"product_name"`
	Category    string                          `bson:"category"`
	Description string                          `bson:"description"`
	Condition   auction_entity.ProductCondition `bson:"condition"`
	Status      auction_entity.AuctionStatus    `bson:"status"`
	Timestamp   int64                           `bson:"timestamp"`
}
type AuctionRepository struct {
	Collection *mongo.Collection
}

func NewAuctionRepository(database *mongo.Database) *AuctionRepository {
	return &AuctionRepository{
		Collection: database.Collection("auctions"),
	}
}

func (ar *AuctionRepository) CreateAuction(
	ctx context.Context,
	auctionEntity *auction_entity.Auction) *internal_error.InternalError {
	auctionEntityMongo := &AuctionEntityMongo{
		Id:          auctionEntity.Id,
		ProductName: auctionEntity.ProductName,
		Category:    auctionEntity.Category,
		Description: auctionEntity.Description,
		Condition:   auctionEntity.Condition,
		Status:      auctionEntity.Status,
		Timestamp:   auctionEntity.Timestamp.Unix(),
	}
	_, err := ar.Collection.InsertOne(ctx, auctionEntityMongo)
	if err != nil {
		logger.Error("Error trying to insert auction", err)
		return internal_error.NewInternalServerError("Error trying to insert auction")
	}

	go ar.closeAuctionAfterDuration(auctionEntity.Id, getAuctionDuration())

	return nil
}

func (ar *AuctionRepository) ScheduleAuctionClosures(ctx context.Context) {
	duration := getAuctionDuration()
	cursor, err := ar.Collection.Find(ctx, bson.M{"status": auction_entity.Active})
	if err != nil {
		logger.Error("Error trying to find active auctions", err)
		return
	}
	defer cursor.Close(ctx)

	var auctions []AuctionEntityMongo
	if err := cursor.All(ctx, &auctions); err != nil {
		logger.Error("Error trying to decode active auctions", err)
		return
	}

	for _, auction := range auctions {
		remaining := duration - time.Since(time.Unix(auction.Timestamp, 0))
		if remaining < 0 {
			remaining = 0
		}
		go ar.closeAuctionAfterDuration(auction.Id, remaining)
	}
}

func (ar *AuctionRepository) closeAuctionAfterDuration(auctionID string, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	<-timer.C
	if err := ar.UpdateAuctionStatus(context.Background(), auctionID, auction_entity.Completed); err != nil {
		logger.Error("Error trying to close auction", err)
	}
}

func (ar *AuctionRepository) UpdateAuctionStatus(
	ctx context.Context, id string, status auction_entity.AuctionStatus) *internal_error.InternalError {
	filter := bson.M{"_id": id, "status": auction_entity.Active}
	update := bson.M{"$set": bson.M{"status": status}}

	if _, err := ar.Collection.UpdateOne(ctx, filter, update); err != nil {
		logger.Error("Error trying to update auction status", err)
		return internal_error.NewInternalServerError("Error trying to update auction status")
	}

	return nil
}

func getAuctionDuration() time.Duration {
	duration, err := time.ParseDuration(os.Getenv("AUCTION_DURATION"))
	if err != nil {
		return 5 * time.Minute
	}

	return duration
}
