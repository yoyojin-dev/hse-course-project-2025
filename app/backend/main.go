package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var boardStages = []string{"ready", "in_progress", "review", "done"}

type Player struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
	TeamID   string `json:"team_id,omitempty"`
	Role     string `json:"role"`
}

type Task struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	TeamID    string `json:"team_id"`
	Stage     string `json:"stage"`
	Blocked   bool   `json:"blocked"`
	OwnerID   string `json:"owner_id,omitempty"`
	Penalty   bool   `json:"penalty,omitempty"`
}

type Team struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	WIPLimit        int                 `json:"wip_limit"`
	Members         []string            `json:"members"`
	Board           map[string][]string `json:"-"`
	CurrentCoin     string              `json:"current_coin,omitempty"`
	TailsNeedsBlock bool                `json:"tails_needs_block,omitempty"`
	TailsBlockDone  bool                `json:"tails_block_done,omitempty"`
	TailsStartDone  bool                `json:"tails_start_done,omitempty"`
}

type ProjectCard struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	TasksByTeam       map[string]int `json:"tasks_by_team"`
	Started           bool           `json:"started"`
	Completed         bool           `json:"completed"`
	StartedDay        int            `json:"started_day,omitempty"`
	DoneDay           int            `json:"done_day,omitempty"`
	TotalTasks        int            `json:"total_tasks"`
	DoneTasks         int            `json:"done_tasks"`
	BoardStage        string         `json:"board_stage"`
	DaysInIntegration int            `json:"days_in_integration,omitempty"`
	PenaltyIssued     bool           `json:"penalty_issued,omitempty"`
}

type LogEntry struct {
	Day      int    `json:"day"`
	Category string `json:"category"`
	Message  string `json:"message"`
	At       string `json:"at"`
}

type Game struct {
	Code            string                  `json:"code"`
	Started         bool                    `json:"started"`
	Finished        bool                    `json:"finished"`
	Phase           string                  `json:"phase"`
	CurrentDay      int                     `json:"current_day"`
	MaxDays         int                     `json:"max_days"`
	CyclesCompleted int                     `json:"cycles_completed"`
	ProjectsDone    int                     `json:"projects_done"`
	Projects        map[string]*ProjectCard `json:"-"`
	ProjectOrder    []string                `json:"-"`
	Teams           map[string]*Team        `json:"-"`
	TeamOrder       []string                `json:"-"`
	Players         map[string]*Player      `json:"-"`
	Tasks           map[string]*Task        `json:"-"`
	FacilitatorID   string                  `json:"facilitator_id"`
	TurnActionDone      map[string]bool         `json:"-"`
	History             []LogEntry              `json:"history"`
}

type Server struct {
	mu            sync.RWMutex
	games         map[string]*Game
	gameCounter   int64
	playerCounter int64
	taskCounter   int64
	rng           *rand.Rand
	wsMu          sync.Mutex
	wsGames       map[string]map[*wsClient]struct{}
}

func newServer() *Server {
	return &Server{
		games:   make(map[string]*Game),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
		wsGames: make(map[string]map[*wsClient]struct{}),
	}
}

type teamState struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	WIPLimit        int               `json:"wip_limit"`
	Members         []Player          `json:"members"`
	Board           map[string][]Task `json:"board"`
	Counts          map[string]int    `json:"counts"`
	CurrentCoin     string            `json:"current_coin,omitempty"`
	TailsNeedsBlock bool              `json:"tails_needs_block,omitempty"`
	TailsBlockDone  bool              `json:"tails_block_done,omitempty"`
	TailsStartDone  bool              `json:"tails_start_done,omitempty"`
}

type projectState struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Started           bool           `json:"started"`
	Completed         bool           `json:"completed"`
	TasksByTeam       map[string]int `json:"tasks_by_team"`
	TotalTasks        int            `json:"total_tasks"`
	DoneTasks         int            `json:"done_tasks"`
	StartedDay        int            `json:"started_day,omitempty"`
	DoneDay           int            `json:"done_day,omitempty"`
	BoardStage        string         `json:"board_stage"`
	DaysInIntegration int            `json:"days_in_integration,omitempty"`
	PenaltyIssued     bool           `json:"penalty_issued,omitempty"`
}

type stateResponse struct {
	Code                string         `json:"code"`
	Started             bool           `json:"started"`
	Finished            bool           `json:"finished"`
	Phase               string         `json:"phase"`
	CurrentDay          int            `json:"current_day"`
	MaxDays             int            `json:"max_days"`
	CurrentTurnTeamID   string         `json:"current_turn_team_id,omitempty"`
	CurrentTurnTeamName string         `json:"current_turn_team_name,omitempty"`
	CyclesCompleted     int            `json:"cycles_completed"`
	ProjectsDone        int            `json:"projects_done"`
	TurnActionDone      map[string]bool `json:"turn_action_done,omitempty"`
	FacilitatorID       string         `json:"facilitator_id"`
	Teams               []teamState    `json:"teams"`
	Projects            []projectState `json:"projects"`
	History             []LogEntry     `json:"history"`
}

type joinRequest struct {
	GameCode string `json:"game_code"`
	Nickname string `json:"nickname"`
	TeamID   string `json:"team_id"`
}

type playerActionRequest struct {
	PlayerID string `json:"player_id"`
}

type createRequest struct {
	TeamNames []string `json:"team_names"`
	MaxDays   int      `json:"max_days"`
}

type startProjectRequest struct {
	PlayerID  string `json:"player_id"`
	ProjectID string `json:"project_id"`
}

type moveTaskRequest struct {
	PlayerID string `json:"player_id"`
	TaskID   string `json:"task_id"`
}

type dragTaskRequest struct {
	PlayerID string `json:"player_id"`
	TaskID   string `json:"task_id"`
	ToStage  string `json:"to_stage"`
}

type setWIPRequest struct {
	PlayerID string `json:"player_id"`
	TeamID   string `json:"team_id"`
	WIPLimit int    `json:"wip_limit"`
}

type lobbyMessage struct {
	Type     string `json:"type"`
	GameCode string `json:"game_code,omitempty"`
}

type lobbyResponse struct {
	Type       string `json:"type"`
	OK         bool   `json:"ok"`
	RedirectTo string `json:"redirect_to,omitempty"`
	Error      string `json:"error,omitempty"`
}

type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

type gameSocketMessage struct {
	Type  string        `json:"type"`
	State stateResponse `json:"state,omitempty"`
	Error string        `json:"error,omitempty"`
}

var lobbyUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var gameUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (c *wsClient) sendJSON(payload interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(payload)
}

func (s *Server) registerGameClient(code string, client *wsClient) {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	if s.wsGames[code] == nil {
		s.wsGames[code] = make(map[*wsClient]struct{})
	}
	s.wsGames[code][client] = struct{}{}
}

func (s *Server) unregisterGameClient(code string, client *wsClient) {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	if s.wsGames[code] == nil {
		return
	}
	delete(s.wsGames[code], client)
	if len(s.wsGames[code]) == 0 {
		delete(s.wsGames, code)
	}
}

func (s *Server) broadcastGameState(code string, state stateResponse) {
	s.wsMu.Lock()
	clients := make([]*wsClient, 0, len(s.wsGames[code]))
	for client := range s.wsGames[code] {
		clients = append(clients, client)
	}
	s.wsMu.Unlock()

	if len(clients) == 0 {
		return
	}

	msg := gameSocketMessage{Type: "state", State: state}
	for _, client := range clients {
		if err := client.sendJSON(msg); err != nil {
			_ = client.conn.Close()
			s.unregisterGameClient(code, client)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errorJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func requestExpectsJSON(r *http.Request) bool {
	accept := strings.ToLower(r.Header.Get("Accept"))
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	return strings.Contains(accept, "application/json") || strings.Contains(ct, "application/json")
}

func parseJSONOrForm(r *http.Request, dst interface{}) error {
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "application/json") {
		return json.NewDecoder(r.Body).Decode(dst)
	}
	if err := r.ParseForm(); err != nil {
		return err
	}
	return nil
}

func nextGameCode() string {
    const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ0123456789"
    code := make([]byte, 6)
    for i := range code {
        code[i] = charset[rand.Intn(len(charset))]
    }
    return string(code)
}

func (s *Server) nextPlayerID() string {
	return "p" + strconv.FormatInt(atomic.AddInt64(&s.playerCounter, 1), 10)
}

func (s *Server) nextTaskID() string {
	return "t" + strconv.FormatInt(atomic.AddInt64(&s.taskCounter, 1), 10)
}

func cloneTasksByTeam(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func preferredTeamNames() []string {
	return []string{"Blue", "Green", "Yellow", "Red", "Purple"}
}

func projectName(i int) string {
	return "Project " + strconv.Itoa(i)
}

func newTeam(id string, name string) *Team {
	board := make(map[string][]string)
	for _, st := range boardStages {
		board[st] = make([]string, 0)
	}
	return &Team{
		ID:       id,
		Name:     name,
		WIPLimit: 2,
		Members:  make([]string, 0),
		Board:    board,
	}
}

func (s *Server) appendLog(g *Game, category string, msg string) {
	g.History = append(g.History, LogEntry{
		Day:      g.CurrentDay,
		Category: category,
		Message:  msg,
		At:       time.Now().Format(time.RFC3339),
	})
}

func (s *Server) makeProjects(teamOrder []string) (map[string]*ProjectCard, []string) {
	projects := make(map[string]*ProjectCard)
	order := make([]string, 0, 15)
	for i := 1; i <= 15; i++ {
		id := fmt.Sprintf("PR-%02d", i)
		tasksByTeam := make(map[string]int)
		total := 0
		for _, teamID := range teamOrder {
			cnt := 1 + s.rng.Intn(3)
			tasksByTeam[teamID] = cnt
			total += cnt
		}
		projects[id] = &ProjectCard{
			ID:          id,
			Name:        projectName(i),
			TasksByTeam: tasksByTeam,
			TotalTasks:  total,
			BoardStage:  "not_started",
		}
		order = append(order, id)
	}
	return projects, order
}

func stateFromGame(g *Game) stateResponse {
	history := make([]LogEntry, len(g.History))
	copy(history, g.History)

	teams := make([]teamState, 0, len(g.TeamOrder))
	for _, teamID := range g.TeamOrder {
		team := g.Teams[teamID]
		members := make([]Player, 0, len(team.Members))
		for _, pid := range team.Members {
			if p, ok := g.Players[pid]; ok {
				members = append(members, *p)
			}
		}
		sort.Slice(members, func(i, j int) bool {
			return members[i].Nickname < members[j].Nickname
		})

		board := make(map[string][]Task)
		counts := make(map[string]int)
		for _, st := range boardStages {
			tasks := make([]Task, 0, len(team.Board[st]))
			for _, tid := range team.Board[st] {
				if task, ok := g.Tasks[tid]; ok {
					tasks = append(tasks, *task)
				}
			}
			board[st] = tasks
			counts[st] = len(tasks)
		}

		teams = append(teams, teamState{
			ID:              team.ID,
			Name:            team.Name,
			WIPLimit:        team.WIPLimit,
			Members:         members,
			Board:           board,
			Counts:          counts,
			CurrentCoin:     team.CurrentCoin,
			TailsNeedsBlock: team.TailsNeedsBlock,
			TailsBlockDone:  team.TailsBlockDone,
			TailsStartDone:  team.TailsStartDone,
		})
	}

	projects := make([]projectState, 0, len(g.ProjectOrder))
	for _, projectID := range g.ProjectOrder {
		p := g.Projects[projectID]
		projects = append(projects, projectState{
			ID:                p.ID,
			Name:              p.Name,
			Started:           p.Started,
			Completed:         p.Completed,
			TasksByTeam:       cloneTasksByTeam(p.TasksByTeam),
			TotalTasks:        p.TotalTasks,
			DoneTasks:         p.DoneTasks,
			StartedDay:        p.StartedDay,
			DoneDay:           p.DoneDay,
			BoardStage:        p.BoardStage,
			DaysInIntegration: p.DaysInIntegration,
			PenaltyIssued:     p.PenaltyIssued,
		})
	}

	turnTeamID := ""
	turnTeamName := ""

	return stateResponse{
		Code:                g.Code,
		Started:             g.Started,
		Finished:            g.Finished,
		Phase:               g.Phase,
		CurrentDay:          g.CurrentDay,
		MaxDays:             g.MaxDays,
		CurrentTurnTeamID:   turnTeamID,
		CurrentTurnTeamName: turnTeamName,
		TurnActionDone:      g.TurnActionDone,
		CyclesCompleted:     g.CyclesCompleted,
		ProjectsDone:        g.ProjectsDone,
		FacilitatorID:       g.FacilitatorID,
		Teams:               teams,
		Projects:            projects,
		History:             history,
	}
}

func (s *Server) findGame(code string) (*Game, bool) {
	g, ok := s.games[code]
	return g, ok
}

func parseJoinRequest(r *http.Request) (joinRequest, error) {
	var req joinRequest
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "application/json") {
		err := json.NewDecoder(r.Body).Decode(&req)
		return req, err
	}
	if err := r.ParseForm(); err != nil {
		return req, err
	}
	req.GameCode = r.FormValue("game_code")
	req.Nickname = r.FormValue("nickname")
	req.TeamID = r.FormValue("team_id")
	return req, nil
}

func splitPathAfter(prefix string, p string) string {
	out := strings.TrimPrefix(p, prefix)
	out = strings.TrimPrefix(out, "/")
	return out
}

func (s *Server) requireFacilitator(g *Game, playerID string) error {
	if playerID == "" {
		return fmt.Errorf("не указан идентификатор ведущего")
	}
	if g.FacilitatorID != playerID {
		return fmt.Errorf("это может сделать только ведущий")
	}
	return nil
}

func (s *Server) ensureRunningTurn(g *Game) {
	if g.Phase != "running" || len(g.TeamOrder) == 0 {
		return
	}
	if g.TurnActionDone == nil {
		g.TurnActionDone = make(map[string]bool)
	}
}

func (s *Server) rollCoinsForTeams(g *Game) {
	for _, team := range g.Teams {
		coin := "tails"
		if s.rng.Intn(2) == 1 {
			coin = "heads"
		}
		team.CurrentCoin = coin
		if coin == "heads" {
			team.TailsNeedsBlock = hasOwnBlockableTask(g, team, "") // wait, heads isn't bound to player anymore
			team.TailsBlockDone = !team.TailsNeedsBlock
			team.TailsStartDone = !hasReadyStartTask(g, team)
			s.appendLog(g, "coin", "Команда "+team.Name+" бросила монетку: heads. Блокировка/старт.")
			if team.TailsBlockDone && team.TailsStartDone {
				g.TurnActionDone[team.ID] = true
			}
		} else {
			team.TailsNeedsBlock = false
			team.TailsBlockDone = false
			team.TailsStartDone = false
			s.appendLog(g, "coin", "Команда "+team.Name+" бросила монетку: tails. Перемещение карточки.")
		}
	}
}

func (s *Server) closeDayAndAdvance(g *Game) {
	if g.CurrentDay >= g.MaxDays {
		g.Finished = true
		g.Phase = "finished"
		s.appendLog(g, "finish", "Игра завершена: достигнут лимит игровых дней.")
		return
	}

	g.CurrentDay++
	s.tickProjectIntegrationDays(g)
	g.TurnActionDone = make(map[string]bool)

	if (g.CurrentDay-1)%5 == 0 {
		g.Phase = "retro"
		g.CyclesCompleted++
		s.appendLog(g, "retro", "Ретро-фаза: обсудите улучшения и при необходимости измените WIP-лимиты.")
		return
	}

	g.Phase = "running"
	s.ensureRunningTurn(g)
	s.rollCoinsForTeams(g)
	s.appendLog(g, "day", "Начался новый игровой день.")
}

func (s *Server) tickProjectIntegrationDays(g *Game) {
	for _, pid := range g.ProjectOrder {
		p := g.Projects[pid]
		if p == nil || !p.Started || p.Completed || p.BoardStage != "integration" {
			continue
		}
		p.DaysInIntegration++
		if p.DaysInIntegration > 5 && !p.PenaltyIssued {
			s.applyProjectIntegrationPenalty(g, p)
		}
	}
}

func (s *Server) applyProjectIntegrationPenalty(g *Game, p *ProjectCard) {
	p.PenaltyIssued = true
	for _, teamID := range g.TeamOrder {
		if p.TasksByTeam[teamID] <= 0 {
			continue
		}
		p.TasksByTeam[teamID]++
		p.TotalTasks++
		tidTask := s.nextTaskID()
		g.Tasks[tidTask] = &Task{ID: tidTask, ProjectID: p.ID, TeamID: teamID, Stage: "ready", Penalty: true}
		g.Teams[teamID].Board["ready"] = append(g.Teams[teamID].Board["ready"], tidTask)
	}
	s.appendLog(g, "project", "Проект "+p.Name+" слишком долго в интеграции: команды получили по штрафной доработке.")
}

func removeTaskFromSlice(items []string, taskID string) []string {
	for i, id := range items {
		if id == taskID {
			return append(items[:i], items[i+1:]...)
		}
	}
	return items
}

func taskBelongsToCurrentTurnTeam(g *Game, player *Player) bool {
	if g.TurnActionDone != nil && g.TurnActionDone[player.TeamID] {
		return false
	}
	return true
}

func firstMovableTask(g *Game, team *Team) *Task {
	for _, st := range []string{"review", "in_progress", "ready"} {
		for _, tid := range team.Board[st] {
			if t, ok := g.Tasks[tid]; ok {
				if t.Blocked {
					continue
				}
				if st == "ready" && len(team.Board["in_progress"]) >= team.WIPLimit {
					continue
				}
				return t
			}
		}
	}
	return nil
}

func (s *Server) moveTaskToStage(g *Game, team *Team, task *Task, to string) bool {
	from := task.Stage
	if from == to {
		return false
	}

	team.Board[from] = removeTaskFromSlice(team.Board[from], task.ID)
	team.Board[to] = append(team.Board[to], task.ID)
	task.Stage = to

	if to == "done" {
		if p, ok := g.Projects[task.ProjectID]; ok && p.Started {
			p.DoneTasks++
			if !p.Completed {
				if (p.BoardStage == "todo" || p.BoardStage == "") && p.DoneTasks == 1 {
					p.BoardStage = "integration"
					s.appendLog(g, "project", "Проект "+p.Name+" перешёл на этап интеграции (первая задача завершена).")
				}
				if p.DoneTasks >= p.TotalTasks {
					p.Completed = true
					p.DoneDay = g.CurrentDay
					p.BoardStage = "done"
					g.ProjectsDone++
					s.appendLog(g, "project", "Проект "+p.Name+" завершён.")
				}
			}
		}
	}

	return true
}

func (s *Server) moveTaskOneStep(g *Game, team *Team, task *Task) string {
	if task.Blocked {
		return ""
	}

	from := task.Stage
	to := ""
	switch from {
	case "ready":
		to = "in_progress"
	case "in_progress":
		to = "review"
	case "review":
		to = "done"
	default:
		return ""
	}

	if !s.moveTaskToStage(g, team, task, to) {
		return ""
	}

	return to
}

func hasOwnHeadsAction(g *Game, team *Team, playerID string) bool {
	for _, st := range []string{"in_progress", "review"} {
		for _, tid := range team.Board[st] {
			t, ok := g.Tasks[tid]
			if !ok {
				continue
			}
			if t.OwnerID != playerID {
				continue
			}
			return true
		}
	}
	return false
}

func hasOwnBlockableTask(g *Game, team *Team, playerID string) bool {
	for _, st := range []string{"in_progress", "review"} {
		for _, tid := range team.Board[st] {
			t, ok := g.Tasks[tid]
			if !ok {
				continue
			}
			if !t.Blocked {
				return true
			}
		}
	}
	return false
}

func hasReadyStartTask(g *Game, team *Team) bool {
	for _, tid := range team.Board["ready"] {
		t, ok := g.Tasks[tid]
		if ok && !t.Blocked {
			return true
		}
	}
	return false
}

func allTeamsDone(g *Game) bool {
	for _, tid := range g.TeamOrder {
		if !g.TurnActionDone[tid] {
			return false
		}
	}
	return true
}

func (s *Server) advanceTurn(g *Game, teamID ...string) {
	if g.TurnActionDone == nil {
		g.TurnActionDone = make(map[string]bool)
	}
	if len(teamID) > 0 && teamID[0] != "" {
		g.TurnActionDone[teamID[0]] = true
		return
	}
	// Backward compatibility for "skip_turn": mark first pending team as done.
	for _, tid := range g.TeamOrder {
		if !g.TurnActionDone[tid] {
			g.TurnActionDone[tid] = true
			return
		}
	}
}

func (s *Server) handleHello(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "hello from backend")
}

func (s *Server) handleLobbyWS(w http.ResponseWriter, r *http.Request) {
	conn, err := lobbyUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	for {
		var msg lobbyMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}

		switch msg.Type {
		case "join_redirect":
			code := strings.TrimSpace(msg.GameCode)
			if code == "" {
				_ = conn.WriteJSON(lobbyResponse{Type: "join_redirect", OK: false, Error: "укажите код игры"})
				continue
			}
			s.mu.RLock()
			_, ok := s.findGame(code)
			s.mu.RUnlock()
			if ok {
				_ = conn.WriteJSON(lobbyResponse{Type: "join_redirect", OK: true, RedirectTo: "/joining/" + code})
			} else {
				_ = conn.WriteJSON(lobbyResponse{Type: "join_redirect", OK: false, Error: "игра не найдена"})
			}
		case "ping":
			_ = conn.WriteJSON(lobbyResponse{Type: "pong", OK: true})
		default:
			_ = conn.WriteJSON(lobbyResponse{Type: msg.Type, OK: false, Error: "неизвестный тип сообщения"})
		}
	}
}

func (s *Server) handleGameWS(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	g, ok := s.findGame(code)
	if !ok {
		s.mu.RUnlock()
		w.WriteHeader(http.StatusNotFound)
		return
	}
	state := stateFromGame(g)
	s.mu.RUnlock()

	conn, err := gameUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &wsClient{conn: conn}
	s.registerGameClient(code, client)

	_ = client.sendJSON(gameSocketMessage{Type: "state", State: state})

	defer func() {
		s.unregisterGameClient(code, client)
		_ = conn.Close()
	}()

	for {
		var msg map[string]string
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		if msg["type"] == "ping" {
			_ = client.sendJSON(gameSocketMessage{Type: "pong"})
		}
	}
}

func (s *Server) handleJoinRedirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	req, err := parseJoinRequest(r)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}

	code := strings.TrimSpace(req.GameCode)
	if code == "" {
		errorJSON(w, http.StatusBadRequest, "укажите код игры")
		return
	}

	s.mu.RLock()
	_, ok := s.findGame(code)
	s.mu.RUnlock()
	if ok {
		http.Redirect(w, r, "/joining/"+code, http.StatusSeeOther)
		return
	}

	cookie := &http.Cookie{
		Name:     "flash",
		Value:    "notfound",
		Path:     "/",
		MaxAge:   5,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleCreateGame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	req := createRequest{TeamNames: []string{"Синяя", "Зеленая", "Желтая", "Красная"}, MaxDays: 15}
	if requestExpectsJSON(r) {
		_ = parseJSONOrForm(r, &req)
	}
	if len(req.TeamNames) < 1 {
		req.TeamNames = []string{"Синяя", "Зеленая", "Желтая", "Красная"}
	}
	if req.MaxDays < 5 {
		req.MaxDays = 15
	}

	code := nextGameCode()
	teams := make(map[string]*Team)
	teamOrder := make([]string, 0, len(req.TeamNames))
	for i, name := range req.TeamNames {
		teamID := "team-" + strconv.Itoa(i+1)
		teams[teamID] = newTeam(teamID, name)
		teamOrder = append(teamOrder, teamID)
	}
	projects, projectOrder := s.makeProjects(teamOrder)

	facilitatorID := s.nextPlayerID()
	facilitator := &Player{ID: facilitatorID, Nickname: "facilitator", Role: "facilitator"}

	game := &Game{
		Code:            code,
		Started:         false,
		Finished:        false,
		Phase:           "setup",
		CurrentDay:      1,
		MaxDays:         req.MaxDays,
		CyclesCompleted: 0,
		ProjectsDone:    0,
		Projects:        projects,
		ProjectOrder:    projectOrder,
		Teams:           teams,
		TeamOrder:       teamOrder,
		Players:         map[string]*Player{facilitatorID: facilitator},
		Tasks:           make(map[string]*Task),
		FacilitatorID:   facilitatorID,
		TurnActionDone:  make(map[string]bool),
		History:         make([]LogEntry, 0),
	}
	s.appendLog(game, "setup", "Игра создана. Ведущий может запускать проекты и управлять раундами.")

	s.mu.Lock()
	s.games[code] = game
	s.mu.Unlock()

	if requestExpectsJSON(r) {
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"game_code":      code,
			"facilitator_id": facilitatorID,
		})
		return
	}

	http.Redirect(w, r, "/created/"+code+"?facilitator_id="+facilitatorID, http.StatusSeeOther)
}

func (s *Server) handleJoinGame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	req, err := parseJoinRequest(r)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}

	code := strings.TrimSpace(req.GameCode)
	nickname := strings.TrimSpace(req.Nickname)
	teamID := strings.TrimSpace(req.TeamID)
	if code == "" || nickname == "" || teamID == "" {
		errorJSON(w, http.StatusBadRequest, "укажите код игры, имя и команду")
		return
	}

	s.mu.Lock()

	g, ok := s.findGame(code)
	if !ok {
		s.mu.Unlock()
		errorJSON(w, http.StatusNotFound, "игра не найдена")
		return
	}
	if g.Started {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "игра уже началась")
		return
	}
	team, teamExists := g.Teams[teamID]
	if !teamExists {
		s.mu.Unlock()
		errorJSON(w, http.StatusBadRequest, "неизвестная команда")
		return
	}
	if len(team.Members) >= 5 {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "команда заполнена (не более 5 участников)")
		return
	}

	for _, existing := range g.Players {
		if strings.EqualFold(existing.Nickname, nickname) {
			state := stateFromGame(g)
			s.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]string{
				"game_code":   g.Code,
				"player_id":   existing.ID,
				"redirect_to": "/game/" + g.Code + "?player_id=" + existing.ID,
			})
			s.broadcastGameState(code, state)
			return
		}
	}

	playerID := s.nextPlayerID()
	p := &Player{ID: playerID, Nickname: nickname, TeamID: teamID, Role: "player"}
	g.Players[playerID] = p
	team.Members = append(team.Members, playerID)
	s.appendLog(g, "join", nickname+" присоединился к команде "+team.Name)
	state := stateFromGame(g)
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, map[string]string{
		"game_code":   g.Code,
		"player_id":   playerID,
		"redirect_to": "/game/" + g.Code + "?player_id=" + playerID,
	})
	s.broadcastGameState(code, state)
}

func (s *Server) handleGetGameState(w http.ResponseWriter, r *http.Request, code string) {
	s.mu.RLock()
	g, ok := s.findGame(code)
	if !ok {
		s.mu.RUnlock()
		errorJSON(w, http.StatusNotFound, "игра не найдена")
		return
	}
	state := stateFromGame(g)
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleStartGame(w http.ResponseWriter, r *http.Request, code string) {
	var req playerActionRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)
	if req.PlayerID == "" {
		errorJSON(w, http.StatusBadRequest, "не указан идентификатор игрока")
		return
	}

	s.mu.Lock()

	g, ok := s.findGame(code)
	if !ok {
		s.mu.Unlock()
		errorJSON(w, http.StatusNotFound, "игра не найдена")
		return
	}
	if err := s.requireFacilitator(g, req.PlayerID); err != nil {
		s.mu.Unlock()
		errorJSON(w, http.StatusForbidden, err.Error())
		return
	}
	if g.Started {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "игра уже началась")
		return
	}
	for _, tid := range g.TeamOrder {
		if len(g.Teams[tid].Members) == 0 {
			s.mu.Unlock()
			errorJSON(w, http.StatusConflict, "в каждой команде должен быть хотя бы один игрок")
			return
		}
	}

	startedAnyProject := false
	for _, pid := range g.ProjectOrder {
		if g.Projects[pid].Started {
			startedAnyProject = true
			break
		}
	}
	if !startedAnyProject {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "сначала запустите хотя бы один проект")
		return
	}

	g.Started = true
	g.Finished = false
	g.TurnActionDone = make(map[string]bool)
	g.Phase = "running"
	s.ensureRunningTurn(g)
	s.rollCoinsForTeams(g)
	s.appendLog(g, "start", "Игра запущена. Все команды ходят одновременно.")
	state := stateFromGame(g)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, state)
	s.broadcastGameState(code, state)
}

func (s *Server) handleStartProject(w http.ResponseWriter, r *http.Request, code string) {
	var req startProjectRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	if req.PlayerID == "" || req.ProjectID == "" {
		errorJSON(w, http.StatusBadRequest, "не указаны идентификатор игрока или проекта")
		return
	}

	s.mu.Lock()

	g, ok := s.findGame(code)
	if !ok {
		s.mu.Unlock()
		errorJSON(w, http.StatusNotFound, "игра не найдена")
		return
	}
	if err := s.requireFacilitator(g, req.PlayerID); err != nil {
		s.mu.Unlock()
		errorJSON(w, http.StatusForbidden, err.Error())
		return
	}
	p, ok := g.Projects[req.ProjectID]
	if !ok {
		s.mu.Unlock()
		errorJSON(w, http.StatusNotFound, "проект не найден")
		return
	}
	if p.Started {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "проект уже запущен")
		return
	}

	anyEmptyReady := false
	for _, tid := range g.TeamOrder {
		if len(g.Teams[tid].Board["ready"]) == 0 {
			anyEmptyReady = true
			break
		}
	}
	if !anyEmptyReady {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "новый проект можно добавить только если колонка «Сделать» пуста хотя бы у одной команды")
		return
	}

	p.Started = true
	p.BoardStage = "todo"
	p.StartedDay = g.CurrentDay
	for _, tid := range g.TeamOrder {
		count := p.TasksByTeam[tid]
		team := g.Teams[tid]
		for i := 0; i < count; i++ {
			tidTask := s.nextTaskID()
			task := &Task{ID: tidTask, ProjectID: p.ID, TeamID: tid, Stage: "ready"}
			g.Tasks[tidTask] = task
			team.Board["ready"] = append(team.Board["ready"], tidTask)
		}
	}

	s.appendLog(g, "project", "Ведущий запустил проект "+p.Name+".")
	state := stateFromGame(g)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, state)
	s.broadcastGameState(code, state)
}

func (s *Server) handleDragTask(w http.ResponseWriter, r *http.Request, code string) {
	var req dragTaskRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)
	req.TaskID = strings.TrimSpace(req.TaskID)
	req.ToStage = strings.TrimSpace(req.ToStage)
	if req.PlayerID == "" || req.TaskID == "" || req.ToStage == "" {
		errorJSON(w, http.StatusBadRequest, "не указаны идентификатор игрока, задачи или целевой колонки")
		return
	}

	s.mu.Lock()
	locked := true
	defer func() {
		if locked {
			s.mu.Unlock()
		}
	}()

	g, ok := s.findGame(code)
	if !ok {
		errorJSON(w, http.StatusNotFound, "игра не найдена")
		return
	}
	if !g.Started {
		errorJSON(w, http.StatusConflict, "игра ещё не начата")
		return
	}
	if g.Finished {
		errorJSON(w, http.StatusConflict, "игра уже завершена")
		return
	}
	if g.Phase != "running" {
		errorJSON(w, http.StatusConflict, "ходы разрешены только в игровой фазе")
		return
	}

	player, ok := g.Players[req.PlayerID]
	if !ok {
		errorJSON(w, http.StatusForbidden, "игрок не в этой игре")
		return
	}
	if player.Role != "player" {
		errorJSON(w, http.StatusForbidden, "ведущий не может двигать карточки")
		return
	}
	if !taskBelongsToCurrentTurnTeam(g, player) {
		errorJSON(w, http.StatusForbidden, "сейчас нельзя ходить вашей команде")
		return
	}
	team := g.Teams[player.TeamID]
	if team.CurrentCoin == "" {
		errorJSON(w, http.StatusConflict, "сначала должен быть бросок монетки")
		return
	}

	task, ok := g.Tasks[req.TaskID]
	if !ok {
		errorJSON(w, http.StatusNotFound, "задача не найдена")
		return
	}
	if task.TeamID != team.ID {
		errorJSON(w, http.StatusForbidden, "задача принадлежит другой команде")
		return
	}

	from := task.Stage
	to := req.ToStage
	if team.CurrentCoin == "tails" {
		needOwnOnly := hasOwnHeadsAction(g, team, player.ID)
		if needOwnOnly && task.Stage != "ready" && task.OwnerID != player.ID {
			errorJSON(w, http.StatusConflict, "сначала поработайте со своими задачами")
			return
		}

		if task.Blocked {
			if from != to {
				errorJSON(w, http.StatusConflict, "заблокированную задачу можно только разблокировать")
				return
			}
			task.Blocked = false
			task.OwnerID = player.ID
			s.appendLog(g, "drag", "Игрок "+player.Nickname+" разблокировал "+task.ID+".")
		} else {
			allowed := false
			switch from {
			case "ready":
				allowed = to == "in_progress"
			case "in_progress":
				allowed = to == "review"
			case "review":
				allowed = to == "done"
			}
			if !allowed {
				// Invalid drag under tails should be a no-op, not a terminal state.
				state := stateFromGame(g)
				locked = false
				s.mu.Unlock()
				writeJSON(w, http.StatusOK, state)
				s.broadcastGameState(code, state)
				return
			}
			if to == "in_progress" && len(team.Board["in_progress"]) >= team.WIPLimit {
				errorJSON(w, http.StatusConflict, "достигнут лимит WIP")
				return
			}
			if !s.moveTaskToStage(g, team, task, to) {
				errorJSON(w, http.StatusConflict, "задачу нельзя переместить")
				return
			}
			if to != "done" {
				task.OwnerID = player.ID
			}
			s.appendLog(g, "drag", "Игрок "+player.Nickname+" перетащил "+task.ID+" из "+from+" в "+to+" (tails).")
		}
		s.advanceTurn(g, team.ID)
	} else {
		if !team.TailsBlockDone {
			if from != to || from == "ready" {
				errorJSON(w, http.StatusConflict, "орёл: сначала заблокируйте задачу в работе или на ревью")
				return
			}
			if task.Blocked || (from != "in_progress" && from != "review") {
				errorJSON(w, http.StatusConflict, "орёл: выберите незаблокированную задачу в работе или на ревью")
				return
			}
			task.Blocked = true
			team.TailsBlockDone = true
			s.appendLog(g, "drag", "Игрок "+player.Nickname+" заблокировал "+task.ID+" (heads).")
		} else if !team.TailsStartDone {
			if from != "ready" || to != "in_progress" {
				errorJSON(w, http.StatusConflict, "орёл: возьмите новую задачу (готово → в работе)")
				return
			}
			if task.Blocked {
				errorJSON(w, http.StatusConflict, "нельзя взять заблокированную задачу")
				return
			}
			if !s.moveTaskToStage(g, team, task, to) {
				errorJSON(w, http.StatusConflict, "задачу нельзя переместить")
				return
			}
			task.OwnerID = player.ID
			team.TailsStartDone = true
			s.appendLog(g, "drag", "Игрок "+player.Nickname+" начал новую задачу "+task.ID+" (heads).")
		} else {
			errorJSON(w, http.StatusConflict, "действия для орла на этот день уже выполнены")
			return
		}

		if team.TailsBlockDone && team.TailsStartDone {
			s.advanceTurn(g, team.ID)
		}
	}

	if g.ProjectsDone == len(g.ProjectOrder) {
		g.Finished = true
		g.Phase = "finished"
		s.appendLog(g, "finish", "Все проекты завершены. Игра окончена.")
	}

	state := stateFromGame(g)
	locked = false
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, state)
	s.broadcastGameState(code, state)
}

func (s *Server) handleSetWIP(w http.ResponseWriter, r *http.Request, code string) {
	var req setWIPRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)
	req.TeamID = strings.TrimSpace(req.TeamID)
	if req.PlayerID == "" || req.TeamID == "" {
		errorJSON(w, http.StatusBadRequest, "не указаны идентификатор игрока или команды")
		return
	}
	if req.WIPLimit < 1 || req.WIPLimit > 10 {
		errorJSON(w, http.StatusBadRequest, "лимит WIP должен быть от 1 до 10")
		return
	}

	s.mu.Lock()
	g, ok := s.findGame(code)
	if !ok {
		s.mu.Unlock()
		errorJSON(w, http.StatusNotFound, "игра не найдена")
		return
	}
	if err := s.requireFacilitator(g, req.PlayerID); err != nil {
		s.mu.Unlock()
		errorJSON(w, http.StatusForbidden, err.Error())
		return
	}
	if g.Phase != "retro" {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "лимит WIP можно менять только на ретро")
		return
	}
	team, ok := g.Teams[req.TeamID]
	if !ok {
		s.mu.Unlock()
		errorJSON(w, http.StatusNotFound, "команда не найдена")
		return
	}
	team.WIPLimit = req.WIPLimit
	s.appendLog(g, "retro", "Изменен WIP лимит команды "+team.Name+" -> "+strconv.Itoa(req.WIPLimit))
	state := stateFromGame(g)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, state)
	s.broadcastGameState(code, state)
}

func (s *Server) handleContinueAfterRetro(w http.ResponseWriter, r *http.Request, code string) {
	var req playerActionRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)
	if req.PlayerID == "" {
		errorJSON(w, http.StatusBadRequest, "не указан идентификатор игрока")
		return
	}

	s.mu.Lock()
	g, ok := s.findGame(code)
	if !ok {
		s.mu.Unlock()
		errorJSON(w, http.StatusNotFound, "игра не найдена")
		return
	}
	if err := s.requireFacilitator(g, req.PlayerID); err != nil {
		s.mu.Unlock()
		errorJSON(w, http.StatusForbidden, err.Error())
		return
	}
	if g.Phase != "retro" {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "сейчас не фаза ретро")
		return
	}

	g.Phase = "running"
	s.ensureRunningTurn(g)
	s.rollCoinsForTeams(g)
	s.appendLog(g, "retro", "Ретро завершено. Игра продолжается.")
	state := stateFromGame(g)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, state)
	s.broadcastGameState(code, state)
}

func (s *Server) handleSkipTurn(w http.ResponseWriter, r *http.Request, code string) {
	var req playerActionRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)
	if req.PlayerID == "" {
		errorJSON(w, http.StatusBadRequest, "не указан идентификатор игрока")
		return
	}

	s.mu.Lock()

	g, ok := s.findGame(code)
	if !ok {
		s.mu.Unlock()
		errorJSON(w, http.StatusNotFound, "игра не найдена")
		return
	}
	if err := s.requireFacilitator(g, req.PlayerID); err != nil {
		s.mu.Unlock()
		errorJSON(w, http.StatusForbidden, err.Error())
		return
	}
	if !g.Started {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "игра ещё не начата")
		return
	}
	if g.Finished {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "игра уже завершена")
		return
	}
	if g.Phase != "running" {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "пропуск хода возможен только в игровой фазе")
		return
	}
	if len(g.TeamOrder) == 0 {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "в игре нет команд")
		return
	}

	s.appendLog(g, "turn", "Ведущий пропустил чей-то ход.")
	s.advanceTurn(g)

	if g.ProjectsDone == len(g.ProjectOrder) {
		g.Finished = true
		g.Phase = "finished"
		s.appendLog(g, "finish", "Все проекты завершены. Игра окончена.")
	}

	state := stateFromGame(g)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, state)
	s.broadcastGameState(code, state)
}

func (s *Server) handleGameRoutes(w http.ResponseWriter, r *http.Request) {
	rest := splitPathAfter("/api/game", r.URL.Path)
	if rest == "" {
		errorJSON(w, http.StatusNotFound, "не найдено")
		return
	}
	parts := strings.Split(rest, "/")
	code := strings.TrimSpace(parts[0])
	if code == "" {
		errorJSON(w, http.StatusBadRequest, "не указан код игры")
		return
	}

	if len(parts) == 1 && r.Method == http.MethodGet {
		s.handleGetGameState(w, r, code)
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPost {
		switch parts[1] {
		case "start":
			s.handleStartGame(w, r, code)
			return
		case "start_project":
			s.handleStartProject(w, r, code)
			return
		case "set_wip":
			s.handleSetWIP(w, r, code)
			return
		case "continue":
			s.handleContinueAfterRetro(w, r, code)
			return
		case "drag":
			s.handleDragTask(w, r, code)
			return
		case "next_day":
			s.handleNextDay(w, r, code)
			return
		case "skip_turn":
			s.handleSkipTurn(w, r, code)
			return
		}
	}

	errorJSON(w, http.StatusNotFound, "не найдено")
}

func main() {
	s := newServer()

	http.HandleFunc("/api/hello", s.handleHello)
	http.HandleFunc("/api/", s.handleJoinRedirect)
	http.HandleFunc("/api/create", s.handleCreateGame)
	http.HandleFunc("/api/join", s.handleJoinGame)
	http.HandleFunc("/api/game/", s.handleGameRoutes)
	http.HandleFunc("/ws/lobby", s.handleLobbyWS)
	http.HandleFunc("/ws/game", s.handleGameWS)

	fmt.Println("Backend started on :8080")
	_ = http.ListenAndServe(":8080", nil)
}

func (s *Server) handleNextDay(w http.ResponseWriter, r *http.Request, code string) {
	var req playerActionRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)

	s.mu.Lock()
	g, ok := s.findGame(code)
	if !ok {
		s.mu.Unlock()
		errorJSON(w, http.StatusNotFound, "игра не найдена")
		return
	}
	if err := s.requireFacilitator(g, req.PlayerID); err != nil {
		s.mu.Unlock()
		errorJSON(w, http.StatusForbidden, err.Error())
		return
	}

	if g.Phase != "running" {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "сейчас не игровая фаза")
		return
	}
	if !allTeamsDone(g) {
		s.mu.Unlock()
		errorJSON(w, http.StatusConflict, "нельзя начать новый день: не все команды завершили действия")
		return
	}

	s.closeDayAndAdvance(g)
	state := stateFromGame(g)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, state)
	s.broadcastGameState(code, state)
}
