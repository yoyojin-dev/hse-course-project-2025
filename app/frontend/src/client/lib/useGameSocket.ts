import { useEffect, useRef, useState } from 'react';
import type { GameState } from '../types';

type StateListener = (state: GameState) => void;

type StatusListener = (connected: boolean) => void;

type SocketMessage = {
  type?: string;
  state?: GameState;
  error?: string;
};

let socket: WebSocket | null = null;
let socketCode = '';
let connected = false;
let closeTimer: number | null = null;
const stateListeners = new Set<StateListener>();
const statusListeners = new Set<StatusListener>();

const notifyStatus = (value: boolean) => {
  connected = value;
  statusListeners.forEach((listener) => listener(value));
};

const scheduleClose = () => {
  if (closeTimer) window.clearTimeout(closeTimer);
  closeTimer = window.setTimeout(() => {
    if (stateListeners.size === 0) {
      socket?.close();
      socket = null;
      socketCode = '';
      notifyStatus(false);
    }
  }, 1200);
};

const ensureSocket = (gameCode: string) => {
  if (!gameCode) return;
  if (socket && socketCode === gameCode) return;

  socket?.close();
  socket = null;
  socketCode = gameCode;

  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
  const nextSocket = new WebSocket(`${protocol}://${window.location.host}/ws/game?code=${encodeURIComponent(gameCode)}`);

  nextSocket.addEventListener('open', () => notifyStatus(true));
  nextSocket.addEventListener('close', () => notifyStatus(false));
  nextSocket.addEventListener('error', () => notifyStatus(false));
  nextSocket.addEventListener('message', (event) => {
    try {
      const payload = JSON.parse(event.data) as SocketMessage;
      if (payload.type === 'state' && payload.state) {
        stateListeners.forEach((listener) => listener(payload.state as GameState));
      }
    } catch {
      // ignore malformed messages
    }
  });

  socket = nextSocket;
};

export const useGameSocket = (gameCode: string, onState: StateListener) => {
  const onStateRef = useRef(onState);
  const [isConnected, setIsConnected] = useState(connected);

  useEffect(() => {
    onStateRef.current = onState;
  }, [onState]);

  useEffect(() => {
    if (!gameCode) return () => undefined;

    const listener: StateListener = (state) => onStateRef.current(state);
    const statusListener: StatusListener = (value) => setIsConnected(value);

    stateListeners.add(listener);
    statusListeners.add(statusListener);
    ensureSocket(gameCode);

    return () => {
      stateListeners.delete(listener);
      statusListeners.delete(statusListener);
      scheduleClose();
    };
  }, [gameCode]);

  return { connected: isConnected };
};
