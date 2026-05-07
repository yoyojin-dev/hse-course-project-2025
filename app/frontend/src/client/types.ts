export type Member = {
  id: string;
  nickname: string;
  role?: string;
  team_id?: string;
};

export type Task = {
  id: string;
  project_id: string;
  stage: string;
  team_id: string;
  blocked?: boolean;
  owner_id?: string;
};

export type Team = {
  id: string;
  name: string;
  wip_limit?: number;
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
};

export type HistoryEntry = {
  day: number;
  category: string;
  message: string;
};

export type GameState = {
  code?: string;
  current_day: number;
  max_days: number;
  projects_done: number;
  cycles_completed: number;
  phase: string;
  turn_action_done?: Record<string, boolean>;
  started?: boolean;
  finished?: boolean;
  facilitator_id?: string;
  teams?: Team[];
  projects?: Project[];
  history?: HistoryEntry[];
};
