package scheduler

import (
	"database/sql"
	"log"
	"strconv"
	"strings"
	"time"
)

// ServerManager defines the operations scheduler needs
type ServerManager interface {
	SendCommand(id, command string) error
	StopServer(id string) error
	StartServer(id string) error
}

type Scheduler struct {
	db      *sql.DB
	manager ServerManager
}

func NewScheduler(db *sql.DB, manager ServerManager) *Scheduler {
	return &Scheduler{db: db, manager: manager}
}

func (s *Scheduler) Start() {
	go s.loop()
	log.Println("Scheduler started")
}

func (s *Scheduler) loop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.check()
	}
}

type scheduledServer struct {
	id       string
	name     string
	schedule string
}

func (s *Scheduler) check() {
	rows, err := s.db.Query(`SELECT id, name, restart_schedule FROM servers WHERE status = 'running' AND restart_schedule != ''`)
	if err != nil {
		return
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var srv scheduledServer
		if err := rows.Scan(&srv.id, &srv.name, &srv.schedule); err != nil {
			continue
		}

		if shouldRestart(srv.schedule, now) {
			log.Printf("Scheduled restart for server %s (%s)", srv.name, srv.id)
			go s.performRestart(srv)
		}
	}
}

func (s *Scheduler) performRestart(srv scheduledServer) {
	// Send warnings
	s.manager.SendCommand(srv.id, "say Server restarting in 5 minutes...")
	time.Sleep(4 * time.Minute)
	s.manager.SendCommand(srv.id, "say Server restarting in 1 minute...")
	time.Sleep(50 * time.Second)
	s.manager.SendCommand(srv.id, "say Server restarting in 10 seconds!")
	time.Sleep(10 * time.Second)

	if err := s.manager.StopServer(srv.id); err != nil {
		log.Printf("Scheduled restart: failed to stop %s: %v", srv.id, err)
		return
	}

	time.Sleep(5 * time.Second)

	if err := s.manager.StartServer(srv.id); err != nil {
		log.Printf("Scheduled restart: failed to start %s: %v", srv.id, err)
	}
}

// shouldRestart checks if the current minute matches the schedule.
// Format: "HH:MM" for daily, "interval:Xh" for every X hours.
func shouldRestart(schedule string, now time.Time) bool {
	schedule = strings.TrimSpace(schedule)

	if len(schedule) == 5 && schedule[2] == ':' {
		parts := strings.Split(schedule, ":")
		if len(parts) == 2 {
			hour, err1 := strconv.Atoi(parts[0])
			minute, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil {
				return now.Hour() == hour && now.Minute() == minute
			}
		}
	}

	if strings.HasPrefix(schedule, "interval:") {
		intervalStr := strings.TrimPrefix(schedule, "interval:")
		intervalStr = strings.TrimSuffix(intervalStr, "h")
		hours, err := strconv.Atoi(intervalStr)
		if err == nil && hours > 0 {
			return now.Minute() == 0 && now.Hour()%hours == 0
		}
	}

	return false
}
