export function parseSSE<T>(buffer: string): { events: T[]; remaining: string };
