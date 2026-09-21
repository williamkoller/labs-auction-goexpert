package auction

import (
	"context"
	"os"
	"testing"
	"time"

	"fullcycle-auction_go/internal/entity/auction_entity"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestCreateAuctionClosesAuctionAfterConfiguredDuration(t *testing.T) {
	os.Setenv("AUCTION_DURATION", "10ms")
	defer os.Unsetenv("AUCTION_DURATION")

	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("closes auction asynchronously", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		mt.AddMockResponses(mtest.CreateSuccessResponse())

		repository := NewAuctionRepository(mt.Client.Database("auctions"))
		auction := &auction_entity.Auction{
			Id:          "auction-id",
			ProductName: "product",
			Category:    "category",
			Description: "valid auction description",
			Condition:   auction_entity.New,
			Status:      auction_entity.Active,
			Timestamp:   time.Now(),
		}

		if err := repository.CreateAuction(context.Background(), auction); err != nil {
			mt.Fatalf("create auction: %v", err)
		}

		waitForCommands(mt, 2, time.Second)

		events := mt.GetAllStartedEvents()
		if events[0].CommandName != "insert" {
			mt.Fatalf("expected insert command, got %s", events[0].CommandName)
		}
		insertedStatus := events[0].Command.Lookup("documents", "0", "status")
		if insertedStatus.Type != bson.TypeInt32 || insertedStatus.Int32() != int32(auction_entity.Active) {
			mt.Fatalf("expected active status on insert, got %v", insertedStatus)
		}

		assertCompletedUpdate(mt, events[1])
	})
}

func TestScheduleAuctionClosuresClosesExpiredAuction(t *testing.T) {
	os.Setenv("AUCTION_DURATION", "10ms")
	defer os.Unsetenv("AUCTION_DURATION")

	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("closes auction whose duration already elapsed", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, "auctions.auctions", mtest.FirstBatch, bson.D{
			{Key: "_id", Value: "auction-id"},
			{Key: "status", Value: auction_entity.Active},
			{Key: "timestamp", Value: time.Now().Add(-time.Hour).Unix()},
		}))
		mt.AddMockResponses(mtest.CreateSuccessResponse())

		repository := NewAuctionRepository(mt.Client.Database("auctions"))
		repository.ScheduleAuctionClosures(context.Background())

		waitForCommands(mt, 2, time.Second)

		events := mt.GetAllStartedEvents()
		if events[0].CommandName != "find" {
			mt.Fatalf("expected find command, got %s", events[0].CommandName)
		}
		assertCompletedUpdate(mt, events[1])
	})
}

func TestScheduleAuctionClosuresWaitsForRemainingDuration(t *testing.T) {
	os.Setenv("AUCTION_DURATION", "2s")
	defer os.Unsetenv("AUCTION_DURATION")

	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("closes auction after the remaining duration", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, "auctions.auctions", mtest.FirstBatch, bson.D{
			{Key: "_id", Value: "auction-id"},
			{Key: "status", Value: auction_entity.Active},
			{Key: "timestamp", Value: time.Now().Unix()},
		}))
		mt.AddMockResponses(mtest.CreateSuccessResponse())

		repository := NewAuctionRepository(mt.Client.Database("auctions"))
		repository.ScheduleAuctionClosures(context.Background())

		earlyDeadline := time.Now().Add(300 * time.Millisecond)
		for time.Now().Before(earlyDeadline) {
			if len(mt.GetAllStartedEvents()) > 1 {
				mt.Fatal("auction closed before the remaining duration elapsed")
			}
			time.Sleep(time.Millisecond)
		}

		waitForCommands(mt, 2, 3*time.Second)
		assertCompletedUpdate(mt, mt.GetAllStartedEvents()[1])
	})
}

func waitForCommands(mt *mtest.T, count int, timeout time.Duration) {
	mt.Helper()
	deadline := time.After(timeout)
	for len(mt.GetAllStartedEvents()) < count || len(mt.GetAllSucceededEvents()) < count {
		select {
		case <-deadline:
			mt.Fatalf("timed out waiting for %d commands; started: %d", count, len(mt.GetAllStartedEvents()))
		case <-time.After(time.Millisecond):
		}
	}
}

func assertCompletedUpdate(mt *mtest.T, request *event.CommandStartedEvent) {
	mt.Helper()
	if request == nil {
		mt.Fatal("expected update command")
	}
	if request.CommandName != "update" {
		mt.Fatalf("expected update command, got %s", request.CommandName)
	}
	update := request.Command.Lookup("updates", "0", "u", "$set", "status")
	if update.Type != bson.TypeInt32 || update.Int32() != int32(auction_entity.Completed) {
		mt.Fatalf("expected completed status update, got %v", update)
	}
}
