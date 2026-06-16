export type Member = {
  id: string;
  nickname: string;
  role?: string;
  team_id?: string;
  current_coin?: string;
};

export type Task = {
  id: string;
  project_id: string;
  stage: string;
  team_id: string;
  blocked?: boolean;
  owner_id?: string;
  penalty?: boolean;
};

export type Team = {
  id: string;
  name: string;
  wip_limit?: number;
  wip_limits?: Record<string, number>;
  members?: Member[];
  counts?: Record<string, number>;
  board?: Record<string, Task[]>;
  current_coin?: string;
  tails_needs_block?: boolean;
  tails_block_done?: boolean;
  tails_start_done?: boolean;
};

export type Project = {
  id: string;
  name: string;
  completed?: boolean;
  started?: boolean;
  done_tasks: number;
  total_tasks: number;
  tasks_by_team?: Record<string, number>;
  board_stage?: string;
  days_in_integration?: number;
  penalty_issued?: boolean;
  started_day?: number;
  done_day?: number;
};

export type HistoryEntry = {
  day: number;
  category: string;
  message: string;
};

export type GameMetrics = {
  cfd: Record<string, number>;
  wip: number;
  lead_time: number;
  blocked: number;
  retro_days: number;
  velocity: number;
  total_blocked_days: number;
  total_penalties: number;
  avg_task_cycle_time: number;
  last_retro_throughput: number;
};

export type GameState = {
  code?: string;
  current_day: number;
  max_days: number;
  projects_done: number;
  cycles_completed: number;
  last_retro_day?: number;
  next_day_is_retro?: boolean;
  project_wip_limits?: Record<string, number>;
  phase: string;
  turn_action_done?: Record<string, boolean>;
  started?: boolean;
  finished?: boolean;
  facilitator_id?: string;
  teams?: Team[];
  projects?: Project[];
  history?: HistoryEntry[];
  metrics?: GameMetrics;
};
