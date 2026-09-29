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

	// Validate the default schedule used for tasks without a custom reminder.
	if _, err := cron.ParseStandard(s.scheduleTime); err != nil {
		slog.Error("invalid SCHEDULE_TIME cron expression, tasks without a custom reminder will not fire", "expr", s.scheduleTime, "error", err)
	}

	loc, _ := time.LoadLocation("Asia/Jakarta")
	s.cron = cron.New(cron.WithLocation(loc))

	// Tick every minute and evaluate each task's own reminder schedule.
	// TODO: Consider using a more efficient approach, such as scheduling each task's next reminder individually, to avoid iterating over all tasks every minute.
	s.cron.AddFunc("* * * * *", func() {
		s.tick(time.Now().In(gmt7))
	})

	s.cron.Start()
	slog.Info("scheduler started", "default_schedule", s.scheduleTime)
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

// tick runs once per minute and sends reminders for every task whose schedule
// is due this minute. now must be in the GMT+7 zone.
func (s *Scheduler) tick(now time.Time) {
	ctx := context.Background()
	activeLists, err := s.db.GetAllActiveLists()
	if err != nil {
		slog.Error("failed to get active lists for pulse", "error", err)
		return
	}

	minuteStart := now.Truncate(time.Minute)
	minuteUnix := minuteStart.Unix()

	for _, list := range activeLists {
		groupJID, err := types.ParseJID(list.GroupJID)
		if err != nil {
			continue
		}

		tasks, err := s.db.GetTasksByList(list.ID)
		if err != nil {
			continue
		}

		var dueTasks []db.Task
		for _, t := range tasks {
			if !t.Reminder || t.Status != "todo" {
				continue
			}
			if t.LastRemindedAt >= minuteUnix {
				continue
			}
			if s.taskDue(t, minuteStart, minuteUnix) {
				dueTasks = append(dueTasks, t)
			}
		}

		if len(dueTasks) == 0 {
			continue
		}

		s.refreshAssigneePresence(ctx, &list)

		text, mentions := format.FormatDailyPulse(&list, dueTasks, now)
		resp, err := s.client.SendGroupMessage(ctx, groupJID, text, mentions)
		if err != nil {
			continue
		}
		s.db.SaveMessageMapping(resp.ID, list.ID, list.GroupJID)
		for _, t := range dueTasks {
			s.db.UpdateTaskLastReminded(t.ID, minuteUnix)
		}
	}
}

// taskDue reports whether the task's reminder schedule fires during the minute
// starting at minuteStart.
func (s *Scheduler) taskDue(t db.Task, minuteStart time.Time, minuteUnix int64) bool {
	if t.ReminderAt > 0 {
		return t.ReminderAt-(t.ReminderAt%60) == minuteUnix
	}

	expr := t.ReminderCron
	if expr == "" {
		expr = s.scheduleTime
	}
	sched, err := cron.ParseStandard(expr)
	if err != nil {
		return false
	}
	return sched.Next(minuteStart.Add(-time.Second)).Equal(minuteStart)
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
