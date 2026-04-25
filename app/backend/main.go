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
}

type Team struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	WIPLimit int                 `json:"wip_limit"`
	Members  []string            `json:"members"`
	Board    map[string][]string `json:"-"`
}

type ProjectCard struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	TasksByTeam map[string]int `json:"tasks_by_team"`
	Started     bool           `json:"started"`
	Completed   bool           `json:"completed"`
	StartedDay  int            `json:"started_day,omitempty"`
	DoneDay     int            `json:"done_day,omitempty"`
	TotalTasks  int            `json:"total_tasks"`
	DoneTasks   int            `json:"done_tasks"`
}

type LogEntry struct {
	Day      int    `json:"day"`
	Category string `json:"category"`
	Message  string `json:"message"`
	At       string `json:"at"`
}

type Game struct {
	Code                string                  `json:"code"`
	Started             bool                    `json:"started"`
	Finished            bool                    `json:"finished"`
	Phase               string                  `json:"phase"`
	CurrentCoin         string                  `json:"current_coin,omitempty"`
	CoinPlayerID        string                  `json:"coin_player_id,omitempty"`
	TailsNeedsBlock     bool                    `json:"tails_needs_block,omitempty"`
	TailsBlockDone      bool                    `json:"tails_block_done,omitempty"`
	TailsStartDone      bool                    `json:"tails_start_done,omitempty"`
	CurrentDay          int                     `json:"current_day"`
	MaxDays             int                     `json:"max_days"`
	CurrentTurnTeamID   string                  `json:"current_turn_team_id,omitempty"`
	CurrentTurnTeamName string                  `json:"current_turn_team_name,omitempty"`
	CyclesCompleted     int                     `json:"cycles_completed"`
	ProjectsDone        int                     `json:"projects_done"`
	Projects            map[string]*ProjectCard `json:"-"`
	ProjectOrder        []string                `json:"-"`
	Teams               map[string]*Team        `json:"-"`
	TeamOrder           []string                `json:"-"`
	Players             map[string]*Player      `json:"-"`
	Tasks               map[string]*Task        `json:"-"`
	FacilitatorID       string                  `json:"facilitator_id"`
	TurnIndex           int                     `json:"-"`
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
}

func newServer() *Server {
	return &Server{
		games: make(map[string]*Game),
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

type teamState struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	WIPLimit int               `json:"wip_limit"`
	Members  []Player          `json:"members"`
	Board    map[string][]Task `json:"board"`
	Counts   map[string]int    `json:"counts"`
}

type projectState struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Started     bool           `json:"started"`
	Completed   bool           `json:"completed"`
	TasksByTeam map[string]int `json:"tasks_by_team"`
	TotalTasks  int            `json:"total_tasks"`
	DoneTasks   int            `json:"done_tasks"`
	StartedDay  int            `json:"started_day,omitempty"`
	DoneDay     int            `json:"done_day,omitempty"`
}

type stateResponse struct {
	Code                string         `json:"code"`
	Started             bool           `json:"started"`
	Finished            bool           `json:"finished"`
	Phase               string         `json:"phase"`
	CurrentCoin         string         `json:"current_coin,omitempty"`
	CoinPlayerID        string         `json:"coin_player_id,omitempty"`
	TailsNeedsBlock     bool           `json:"tails_needs_block,omitempty"`
	TailsBlockDone      bool           `json:"tails_block_done,omitempty"`
	TailsStartDone      bool           `json:"tails_start_done,omitempty"`
	CurrentDay          int            `json:"current_day"`
	MaxDays             int            `json:"max_days"`
	CurrentTurnTeamID   string         `json:"current_turn_team_id,omitempty"`
	CurrentTurnTeamName string         `json:"current_turn_team_name,omitempty"`
	CyclesCompleted     int            `json:"cycles_completed"`
	ProjectsDone        int            `json:"projects_done"`
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
	TeamCount int `json:"team_count"`
	MaxDays   int `json:"max_days"`
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

func (s *Server) nextGameCode() string {
	return fmt.Sprintf("%06d", atomic.AddInt64(&s.gameCounter, 1))
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
			ID:       team.ID,
			Name:     team.Name,
			WIPLimit: team.WIPLimit,
			Members:  members,
			Board:    board,
			Counts:   counts,
		})
	}

	projects := make([]projectState, 0, len(g.ProjectOrder))
	for _, projectID := range g.ProjectOrder {
		p := g.Projects[projectID]
		projects = append(projects, projectState{
			ID:          p.ID,
			Name:        p.Name,
			Started:     p.Started,
			Completed:   p.Completed,
			TasksByTeam: cloneTasksByTeam(p.TasksByTeam),
			TotalTasks:  p.TotalTasks,
			DoneTasks:   p.DoneTasks,
			StartedDay:  p.StartedDay,
			DoneDay:     p.DoneDay,
		})
	}

	turnTeamID := ""
	turnTeamName := ""
	if g.Started && !g.Finished && len(g.TeamOrder) > 0 && g.Phase == "running" {
		turnTeamID = g.TeamOrder[g.TurnIndex]
		if t, ok := g.Teams[turnTeamID]; ok {
			turnTeamName = t.Name
		}
	}

	return stateResponse{
		Code:                g.Code,
		Started:             g.Started,
		Finished:            g.Finished,
		Phase:               g.Phase,
		CurrentCoin:         g.CurrentCoin,
		CoinPlayerID:        g.CoinPlayerID,
		TailsNeedsBlock:     g.TailsNeedsBlock,
		TailsBlockDone:      g.TailsBlockDone,
		TailsStartDone:      g.TailsStartDone,
		CurrentDay:          g.CurrentDay,
		MaxDays:             g.MaxDays,
		CurrentTurnTeamID:   turnTeamID,
		CurrentTurnTeamName: turnTeamName,
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
		return fmt.Errorf("missing player_id")
	}
	if g.FacilitatorID != playerID {
		return fmt.Errorf("only facilitator can do this")
	}
	return nil
}

func (s *Server) ensureRunningTurn(g *Game) {
	if g.Phase != "running" || len(g.TeamOrder) == 0 {
		g.CurrentTurnTeamID = ""
		g.CurrentTurnTeamName = ""
		return
	}
	if g.TurnIndex >= len(g.TeamOrder) {
		g.TurnIndex = 0
	}
	tid := g.TeamOrder[g.TurnIndex]
	g.CurrentTurnTeamID = tid
	if t, ok := g.Teams[tid]; ok {
		g.CurrentTurnTeamName = t.Name
	}
}

func (s *Server) resetCoinState(g *Game) {
	g.CurrentCoin = ""
	g.CoinPlayerID = ""
	g.TailsNeedsBlock = false
	g.TailsBlockDone = false
	g.TailsStartDone = false
}

func (s *Server) closeDayAndAdvance(g *Game) {
	if g.CurrentDay >= g.MaxDays {
		g.Finished = true
		g.Phase = "finished"
		s.appendLog(g, "finish", "Игра завершена: достигнут лимит игровых дней.")
		return
	}

	g.CurrentDay++
	g.TurnActionDone = make(map[string]bool)
	g.TurnIndex = 0
	s.resetCoinState(g)

	if (g.CurrentDay-1)%5 == 0 {
		g.Phase = "retro"
		g.CyclesCompleted++
		s.appendLog(g, "retro", "Ретро-фаза: обсудите улучшения и при необходимости измените WIP-лимиты.")
		g.CurrentTurnTeamID = ""
		g.CurrentTurnTeamName = ""
		return
	}

	g.Phase = "running"
	s.ensureRunningTurn(g)
	s.appendLog(g, "day", "Начался новый игровой день.")
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
	if g.CurrentTurnTeamID == "" {
		return false
	}
	return player.TeamID == g.CurrentTurnTeamID
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
		if p, ok := g.Projects[task.ProjectID]; ok {
			p.DoneTasks++
			if !p.Completed && p.DoneTasks >= p.TotalTasks {
				p.Completed = true
				p.DoneDay = g.CurrentDay
				g.ProjectsDone++
				s.appendLog(g, "project", "Проект "+p.Name+" завершен.")
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
			if t.OwnerID == playerID && !t.Blocked {
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

func (s *Server) advanceTurn(g *Game) {
	s.resetCoinState(g)
	g.TurnActionDone[g.CurrentTurnTeamID] = true
	if len(g.TeamOrder) == 0 {
		return
	}

	for i := 0; i < len(g.TeamOrder); i++ {
		g.TurnIndex = (g.TurnIndex + 1) % len(g.TeamOrder)
		tid := g.TeamOrder[g.TurnIndex]
		if !g.TurnActionDone[tid] {
			s.ensureRunningTurn(g)
			return
		}
	}

	s.closeDayAndAdvance(g)
}

func (s *Server) handleHello(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "hello from backend")
}

func (s *Server) handleJoinRedirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	req, err := parseJoinRequest(r)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request")
		return
	}

	code := strings.TrimSpace(req.GameCode)
	if code == "" {
		errorJSON(w, http.StatusBadRequest, "missing game_code")
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

	req := createRequest{TeamCount: 4, MaxDays: 15}
	if requestExpectsJSON(r) {
		_ = parseJSONOrForm(r, &req)
	}
	if req.TeamCount < 3 {
		req.TeamCount = 3
	}
	if req.TeamCount > 5 {
		req.TeamCount = 5
	}
	if req.MaxDays < 5 {
		req.MaxDays = 15
	}

	code := s.nextGameCode()
	teamNames := preferredTeamNames()
	teams := make(map[string]*Team)
	teamOrder := make([]string, 0, req.TeamCount)
	for i := 0; i < req.TeamCount; i++ {
		teamID := "team-" + strconv.Itoa(i+1)
		teams[teamID] = newTeam(teamID, teamNames[i])
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
		TurnIndex:       0,
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
		errorJSON(w, http.StatusBadRequest, "invalid request")
		return
	}

	code := strings.TrimSpace(req.GameCode)
	nickname := strings.TrimSpace(req.Nickname)
	teamID := strings.TrimSpace(req.TeamID)
	if code == "" || nickname == "" || teamID == "" {
		errorJSON(w, http.StatusBadRequest, "missing game_code, nickname or team_id")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.findGame(code)
	if !ok {
		errorJSON(w, http.StatusNotFound, "game not found")
		return
	}
	if g.Started {
		errorJSON(w, http.StatusConflict, "game already started")
		return
	}
	team, teamExists := g.Teams[teamID]
	if !teamExists {
		errorJSON(w, http.StatusBadRequest, "unknown team")
		return
	}
	if len(team.Members) >= 5 {
		errorJSON(w, http.StatusConflict, "team is full (max 5)")
		return
	}

	for _, existing := range g.Players {
		if strings.EqualFold(existing.Nickname, nickname) {
			writeJSON(w, http.StatusOK, map[string]string{
				"game_code":   g.Code,
				"player_id":   existing.ID,
				"redirect_to": "/game/" + g.Code + "?player_id=" + existing.ID,
			})
			return
		}
	}

	playerID := s.nextPlayerID()
	p := &Player{ID: playerID, Nickname: nickname, TeamID: teamID, Role: "player"}
	g.Players[playerID] = p
	team.Members = append(team.Members, playerID)
	s.appendLog(g, "join", nickname+" присоединился к команде "+team.Name)

	writeJSON(w, http.StatusCreated, map[string]string{
		"game_code":   g.Code,
		"player_id":   playerID,
		"redirect_to": "/game/" + g.Code + "?player_id=" + playerID,
	})
}

func (s *Server) handleGetGameState(w http.ResponseWriter, r *http.Request, code string) {
	s.mu.RLock()
	g, ok := s.findGame(code)
	if !ok {
		s.mu.RUnlock()
		errorJSON(w, http.StatusNotFound, "game not found")
		return
	}
	state := stateFromGame(g)
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleStartGame(w http.ResponseWriter, r *http.Request, code string) {
	var req playerActionRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)
	if req.PlayerID == "" {
		errorJSON(w, http.StatusBadRequest, "missing player_id")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.findGame(code)
	if !ok {
		errorJSON(w, http.StatusNotFound, "game not found")
		return
	}
	if err := s.requireFacilitator(g, req.PlayerID); err != nil {
		errorJSON(w, http.StatusForbidden, err.Error())
		return
	}
	if g.Started {
		errorJSON(w, http.StatusConflict, "game already started")
		return
	}
	for _, tid := range g.TeamOrder {
		if len(g.Teams[tid].Members) == 0 {
			errorJSON(w, http.StatusConflict, "each team must have at least 1 player")
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
		errorJSON(w, http.StatusConflict, "start at least one project first")
		return
	}

	g.Started = true
	g.Finished = false
	g.TurnIndex = 0
	g.TurnActionDone = make(map[string]bool)
	g.Phase = "running"
	s.ensureRunningTurn(g)
	s.appendLog(g, "start", "Игра запущена. Ходы выполняются по очереди команд.")

	writeJSON(w, http.StatusOK, stateFromGame(g))
}

func (s *Server) handleStartProject(w http.ResponseWriter, r *http.Request, code string) {
	var req startProjectRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	if req.PlayerID == "" || req.ProjectID == "" {
		errorJSON(w, http.StatusBadRequest, "missing player_id or project_id")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.findGame(code)
	if !ok {
		errorJSON(w, http.StatusNotFound, "game not found")
		return
	}
	if err := s.requireFacilitator(g, req.PlayerID); err != nil {
		errorJSON(w, http.StatusForbidden, err.Error())
		return
	}
	p, ok := g.Projects[req.ProjectID]
	if !ok {
		errorJSON(w, http.StatusNotFound, "project not found")
		return
	}
	if p.Started {
		errorJSON(w, http.StatusConflict, "project already started")
		return
	}

	p.Started = true
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
	writeJSON(w, http.StatusOK, stateFromGame(g))
}

func (s *Server) handleMoveTask(w http.ResponseWriter, r *http.Request, code string) {
	var req moveTaskRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)
	req.TaskID = strings.TrimSpace(req.TaskID)
	if req.PlayerID == "" {
		errorJSON(w, http.StatusBadRequest, "missing player_id")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.findGame(code)
	if !ok {
		errorJSON(w, http.StatusNotFound, "game not found")
		return
	}
	if !g.Started {
		errorJSON(w, http.StatusConflict, "game is not started")
		return
	}
	if g.Finished {
		errorJSON(w, http.StatusConflict, "game is already finished")
		return
	}
	if g.Phase != "running" {
		errorJSON(w, http.StatusConflict, "moves are allowed only during running phase")
		return
	}

	player, ok := g.Players[req.PlayerID]
	if !ok {
		errorJSON(w, http.StatusForbidden, "player is not in game")
		return
	}
	if player.Role != "player" {
		errorJSON(w, http.StatusForbidden, "facilitator cannot make team moves")
		return
	}
	if !taskBelongsToCurrentTurnTeam(g, player) {
		errorJSON(w, http.StatusForbidden, "not your team turn")
		return
	}
	if g.CurrentCoin != "" {
		errorJSON(w, http.StatusConflict, "coin already tossed for this turn")
		return
	}

	team := g.Teams[player.TeamID]
	_ = req.TaskID

	coin := "tails"
	if s.rng.Intn(2) == 1 {
		coin = "heads"
	}
	g.CurrentCoin = coin
	g.CoinPlayerID = player.ID

	if coin == "heads" {
		g.TailsNeedsBlock = hasOwnBlockableTask(g, team, player.ID)
		g.TailsBlockDone = !g.TailsNeedsBlock
		g.TailsStartDone = !hasReadyStartTask(g, team)
		s.appendLog(g, "coin", "Команда "+team.Name+" бросила монетку: heads. Выполните: блокировка своей работы (если есть) и старт новой ready-задачи.")
		if g.TailsStartDone {
			s.appendLog(g, "coin", "Команда "+team.Name+": нет доступных ready-задач для старта.")
		}
		if g.TailsBlockDone {
			s.appendLog(g, "coin", "Команда "+team.Name+": своих задач для блокировки нет.")
		}
		if g.TailsBlockDone && g.TailsStartDone {
			s.advanceTurn(g)
		}
	} else {
		s.appendLog(g, "coin", "Команда "+team.Name+" бросила монетку: tails. Выполните действие перетаскиванием карточки.")
	}

	writeJSON(w, http.StatusOK, stateFromGame(g))
}

func (s *Server) handleDragTask(w http.ResponseWriter, r *http.Request, code string) {
	var req dragTaskRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)
	req.TaskID = strings.TrimSpace(req.TaskID)
	req.ToStage = strings.TrimSpace(req.ToStage)
	if req.PlayerID == "" || req.TaskID == "" || req.ToStage == "" {
		errorJSON(w, http.StatusBadRequest, "missing player_id, task_id or to_stage")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.findGame(code)
	if !ok {
		errorJSON(w, http.StatusNotFound, "game not found")
		return
	}
	if !g.Started {
		errorJSON(w, http.StatusConflict, "game is not started")
		return
	}
	if g.Finished {
		errorJSON(w, http.StatusConflict, "game is already finished")
		return
	}
	if g.Phase != "running" {
		errorJSON(w, http.StatusConflict, "moves are allowed only during running phase")
		return
	}

	player, ok := g.Players[req.PlayerID]
	if !ok {
		errorJSON(w, http.StatusForbidden, "player is not in game")
		return
	}
	if player.Role != "player" {
		errorJSON(w, http.StatusForbidden, "facilitator cannot move tasks")
		return
	}
	if !taskBelongsToCurrentTurnTeam(g, player) {
		errorJSON(w, http.StatusForbidden, "not your team turn")
		return
	}
	if g.CurrentCoin == "" {
		errorJSON(w, http.StatusConflict, "toss coin first")
		return
	}
	if g.CoinPlayerID != player.ID {
		errorJSON(w, http.StatusForbidden, "only the player who tossed the coin can act")
		return
	}

	team := g.Teams[player.TeamID]
	task, ok := g.Tasks[req.TaskID]
	if !ok {
		errorJSON(w, http.StatusNotFound, "task not found")
		return
	}
	if task.TeamID != team.ID {
		errorJSON(w, http.StatusForbidden, "task belongs to another team")
		return
	}

	from := task.Stage
	to := req.ToStage
	if g.CurrentCoin == "tails" {
		needOwnOnly := hasOwnHeadsAction(g, team, player.ID)
		if needOwnOnly && task.Stage != "ready" && task.OwnerID != player.ID {
			errorJSON(w, http.StatusConflict, "you must first work with your own tasks")
			return
		}

		if task.Blocked {
			if from != to {
				errorJSON(w, http.StatusConflict, "blocked task can only be unblocked")
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
				errorJSON(w, http.StatusConflict, "invalid stage transition for tails")
				return
			}
			if to == "in_progress" && len(team.Board["in_progress"]) >= team.WIPLimit {
				errorJSON(w, http.StatusConflict, "WIP limit reached")
				return
			}
			if !s.moveTaskToStage(g, team, task, to) {
				errorJSON(w, http.StatusConflict, "cannot move task")
				return
			}
			if to != "done" {
				task.OwnerID = player.ID
			}
			s.appendLog(g, "drag", "Игрок "+player.Nickname+" перетащил "+task.ID+" из "+from+" в "+to+" (tails).")
		}
		s.advanceTurn(g)
	} else {
		if !g.TailsBlockDone {
			if from != to || from == "ready" {
				errorJSON(w, http.StatusConflict, "heads: first block your own in_progress/review task")
				return
			}
			if task.OwnerID != player.ID || task.Blocked || (from != "in_progress" && from != "review") {
				errorJSON(w, http.StatusConflict, "heads: choose your own unblocked in_progress/review task")
				return
			}
			task.Blocked = true
			g.TailsBlockDone = true
			s.appendLog(g, "drag", "Игрок "+player.Nickname+" заблокировал "+task.ID+" (heads).")
		} else if !g.TailsStartDone {
			if from != "ready" || to != "in_progress" {
				errorJSON(w, http.StatusConflict, "heads: now start new task (ready -> in_progress)")
				return
			}
			if task.Blocked {
				errorJSON(w, http.StatusConflict, "cannot start blocked task")
				return
			}
			if !s.moveTaskToStage(g, team, task, to) {
				errorJSON(w, http.StatusConflict, "cannot move task")
				return
			}
			task.OwnerID = player.ID
			g.TailsStartDone = true
			s.appendLog(g, "drag", "Игрок "+player.Nickname+" начал новую задачу "+task.ID+" (heads).")
		} else {
			errorJSON(w, http.StatusConflict, "heads actions already completed")
			return
		}

		if g.TailsBlockDone && g.TailsStartDone {
			s.advanceTurn(g)
		}
	}

	if g.ProjectsDone == len(g.ProjectOrder) {
		g.Finished = true
		g.Phase = "finished"
		s.appendLog(g, "finish", "Все проекты завершены. Игра окончена.")
	}

	writeJSON(w, http.StatusOK, stateFromGame(g))
}

func (s *Server) handleSetWIP(w http.ResponseWriter, r *http.Request, code string) {
	var req setWIPRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)
	req.TeamID = strings.TrimSpace(req.TeamID)
	if req.PlayerID == "" || req.TeamID == "" {
		errorJSON(w, http.StatusBadRequest, "missing player_id or team_id")
		return
	}
	if req.WIPLimit < 1 || req.WIPLimit > 10 {
		errorJSON(w, http.StatusBadRequest, "wip_limit must be in range 1..10")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.findGame(code)
	if !ok {
		errorJSON(w, http.StatusNotFound, "game not found")
		return
	}
	if err := s.requireFacilitator(g, req.PlayerID); err != nil {
		errorJSON(w, http.StatusForbidden, err.Error())
		return
	}
	if g.Phase != "retro" {
		errorJSON(w, http.StatusConflict, "WIP can be changed only during retro phase")
		return
	}
	team, ok := g.Teams[req.TeamID]
	if !ok {
		errorJSON(w, http.StatusNotFound, "team not found")
		return
	}
	team.WIPLimit = req.WIPLimit
	s.appendLog(g, "retro", "Изменен WIP лимит команды "+team.Name+" -> "+strconv.Itoa(req.WIPLimit))
	writeJSON(w, http.StatusOK, stateFromGame(g))
}

func (s *Server) handleContinueAfterRetro(w http.ResponseWriter, r *http.Request, code string) {
	var req playerActionRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)
	if req.PlayerID == "" {
		errorJSON(w, http.StatusBadRequest, "missing player_id")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.findGame(code)
	if !ok {
		errorJSON(w, http.StatusNotFound, "game not found")
		return
	}
	if err := s.requireFacilitator(g, req.PlayerID); err != nil {
		errorJSON(w, http.StatusForbidden, err.Error())
		return
	}
	if g.Phase != "retro" {
		errorJSON(w, http.StatusConflict, "game is not in retro phase")
		return
	}

	g.Phase = "running"
	g.TurnActionDone = make(map[string]bool)
	g.TurnIndex = 0
	s.ensureRunningTurn(g)
	s.appendLog(g, "retro", "Ретро завершено. Игра продолжается.")
	writeJSON(w, http.StatusOK, stateFromGame(g))
}

func (s *Server) handleSkipTurn(w http.ResponseWriter, r *http.Request, code string) {
	var req playerActionRequest
	if err := parseJSONOrForm(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.PlayerID = strings.TrimSpace(req.PlayerID)
	if req.PlayerID == "" {
		errorJSON(w, http.StatusBadRequest, "missing player_id")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.findGame(code)
	if !ok {
		errorJSON(w, http.StatusNotFound, "game not found")
		return
	}
	if err := s.requireFacilitator(g, req.PlayerID); err != nil {
		errorJSON(w, http.StatusForbidden, err.Error())
		return
	}
	if !g.Started {
		errorJSON(w, http.StatusConflict, "game is not started")
		return
	}
	if g.Finished {
		errorJSON(w, http.StatusConflict, "game is already finished")
		return
	}
	if g.Phase != "running" {
		errorJSON(w, http.StatusConflict, "skip is allowed only during running phase")
		return
	}
	if len(g.TeamOrder) == 0 {
		errorJSON(w, http.StatusConflict, "no teams in game")
		return
	}

	tid := g.CurrentTurnTeamID
	tname := tid
	if t, ok := g.Teams[tid]; ok {
		tname = t.Name
	}
	s.appendLog(g, "turn", "Ведущий пропустил ход команды "+tname+".")
	s.advanceTurn(g)

	if g.ProjectsDone == len(g.ProjectOrder) {
		g.Finished = true
		g.Phase = "finished"
		s.appendLog(g, "finish", "Все проекты завершены. Игра окончена.")
	}

	writeJSON(w, http.StatusOK, stateFromGame(g))
}

func (s *Server) handleGameRoutes(w http.ResponseWriter, r *http.Request) {
	rest := splitPathAfter("/api/game", r.URL.Path)
	if rest == "" {
		errorJSON(w, http.StatusNotFound, "not found")
		return
	}
	parts := strings.Split(rest, "/")
	code := strings.TrimSpace(parts[0])
	if code == "" {
		errorJSON(w, http.StatusBadRequest, "missing game code")
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
		case "move":
			s.handleMoveTask(w, r, code)
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
		case "skip_turn":
			s.handleSkipTurn(w, r, code)
			return
		}
	}

	errorJSON(w, http.StatusNotFound, "not found")
}

func main() {
	s := newServer()

	http.HandleFunc("/api/hello", s.handleHello)
	http.HandleFunc("/api/", s.handleJoinRedirect)
	http.HandleFunc("/api/create", s.handleCreateGame)
	http.HandleFunc("/api/join", s.handleJoinGame)
	http.HandleFunc("/api/game/", s.handleGameRoutes)

	fmt.Println("Backend started on :8080")
	_ = http.ListenAndServe(":8080", nil)
}
