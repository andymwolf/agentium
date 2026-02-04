import { Request, Response, NextFunction } from 'express';

/**
 * API key authentication middleware
 *
 * Validates the Authorization header contains a valid Bearer token.
 * API keys should be configured via AGENTIUM_API_KEYS environment variable
 * as a comma-separated list of valid keys.
 */
export function apiKeyAuth(req: Request, res: Response, next: NextFunction): void {
  const authHeader = req.headers.authorization;

  if (!authHeader) {
    res.status(401).json({
      error: 'Unauthorized',
      message: 'Missing Authorization header',
    });
    return;
  }

  if (!authHeader.startsWith('Bearer ')) {
    res.status(401).json({
      error: 'Unauthorized',
      message: 'Invalid Authorization header format. Expected: Bearer <token>',
    });
    return;
  }

  const token = authHeader.slice(7); // Remove 'Bearer ' prefix

  if (!token) {
    res.status(401).json({
      error: 'Unauthorized',
      message: 'Missing API key',
    });
    return;
  }

  const validApiKeys = getValidApiKeys();

  if (!validApiKeys.includes(token)) {
    res.status(401).json({
      error: 'Unauthorized',
      message: 'Invalid API key',
    });
    return;
  }

  next();
}

/**
 * Get the list of valid API keys from environment
 */
function getValidApiKeys(): string[] {
  const keys = process.env.AGENTIUM_API_KEYS || '';
  return keys
    .split(',')
    .map((key) => key.trim())
    .filter((key) => key.length > 0);
}
