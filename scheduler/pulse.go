package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
	"go.mau.fi/whatsmeow/types"

	"pm-wa/bot"
	"pm-wa/db"
	"pm-wa/format"
)

var gmt7 = time.FixedZone("GMT+7", 7*60*60)

type Scheduler struct {
	client *bot.Client
	db     *db.Database
	cron   *cron.Cron
}

func New(client *bot.Client, database *db.Database) *Scheduler {
	return &Scheduler{
		client: client,
		db:     database,
	}
}

func (s *Scheduler) Start() {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	s.cron = cron.New(cron.WithLocation(loc))

	s.cron.AddFunc("0 8 * * *", func() {
		s.executeDailyPulse()
	})

	s.cron.Start()
	slog.Info("scheduler started", "next_pulse", "08:00 AM GMT+7")
}

func (s *Scheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
	}
}

func (s *Scheduler) executeDailyPulse() {
	ctx := context.Background()
	activeLists, err := s.db.GetAllActiveLists()
	if err != nil {
		slog.Error("failed to get active lists for pulse", "error", err)
		return
	}

	for _, list := range activeLists {
		groupJID, err := types.ParseJID(list.GroupJID)
		if err != nil {
			continue
		}

		inGroup, _ := s.client.IsBotInGroup(ctx, groupJID)
		if !inGroup {
			s.handleBotKicked(ctx, groupJID)
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
