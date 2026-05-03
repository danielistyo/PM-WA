package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.mau.fi/whatsmeow/types"

	"pm-wa/bot"
	"pm-wa/db"
	"pm-wa/format"
)

var gmt7 = time.FixedZone("GMT+7", 7*60*60)

type Scheduler struct {
	client       *bot.Client
	db           *db.Database
	cron         *cron.Cron
	mu           sync.Mutex
	scheduleTime string
}

func New(client *bot.Client, database *db.Database, scheduleTime string) *Scheduler {
	return &Scheduler{
		client:       client,
		db:           database,
		scheduleTime: scheduleTime,
	}
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cron != nil {
		// Already running; skip to prevent duplicate registrations
		slog.Info("scheduler already running, skipping Start")
		return
	}

	// Validate the cron expression before starting
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(s.scheduleTime); err != nil {
		slog.Error("invalid SCHEDULE_TIME cron expression, scheduler not started", "expr", s.scheduleTime, "error", err)
		return
	}

	loc, _ := time.LoadLocation("Asia/Jakarta")
	s.cron = cron.New(cron.WithLocation(loc))

	s.cron.AddFunc(s.scheduleTime, func() {
		s.executeDailyPulse()
	})

	s.cron.Start()
	slog.Info("scheduler started", "schedule", s.scheduleTime)
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cron != nil {
		ctx := s.cron.Stop()
		<-ctx.Done()
		s.cron = nil
	}
}

func (s *Scheduler) executeDailyPulse() {
	ctx := context.Background()
	activeLists, err := s.db.GetAllActiveLists()
	if err != nil {
		slog.Error("failed to get active lists for pulse", "error", err)
		return
	}

	today := time.Now().In(gmt7).Format("2006-01-02")

	for _, list := range activeLists {
		groupJID, err := types.ParseJID(list.GroupJID)
		if err != nil {
			continue
		}

		// Skip if a reminder was already sent today (WIB) — guards against duplicate
		// sends on app restart or multiple Connected events.
		if list.LastRemindedDate == today {
			slog.Info("reminder already sent today, skipping", "list_id", list.ID, "date", today)
			continue
		}

		s.refreshAssigneePresence(ctx, &list)

		tasks, err := s.db.GetTasksByList(list.ID)
		if err != nil {
			continue
		}

		allDone := true
		for _, t := range tasks {
			if t.Status == "todo" {
				allDone = false
				break
			}
		}
		if allDone {
			continue
		}

		var pulseTasks []db.Task
		for _, t := range tasks {
			if t.Reminder && t.Status == "todo" {
				pulseTasks = append(pulseTasks, t)
			}
		}

		if len(pulseTasks) == 0 {
			continue
		}

		now := time.Now().In(gmt7)
		text, mentions := format.FormatDailyPulse(&list, pulseTasks, now)
		resp, err := s.client.SendGroupMessage(ctx, groupJID, text, mentions)
		if err == nil {
			s.db.SaveMessageMapping(resp.ID, list.ID, list.GroupJID)
			s.db.UpdateLastRemindedDate(list.ID, today)
		}
	}
}

func (s *Scheduler) handleBotKicked(ctx context.Context, groupJID types.JID) {
	lists, _ := s.db.GetActiveListsByGroup(groupJID.String())
	groupName := s.db.GetGroupName(groupJID.String())

	for _, list := range lists {
		s.db.UpdateListStatus(list.ID, "stopped")
		adminJID, err := types.ParseJID(list.AdminJID)
		if err != nil {
			continue
		}
		msg := "Alert: I was kicked from '" + groupName + "'. All task lists for this group have been permanently stopped."
		s.client.SendPM(ctx, adminJID, msg)
	}

	s.db.DeleteGroup(groupJID.String())
}
