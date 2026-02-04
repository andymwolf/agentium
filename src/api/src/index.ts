import express, { Express, Request, Response, NextFunction } from 'express';
import { tasksRouter } from './routes/tasks.js';

/**
 * Create and configure the Express application
 */
export function createApp(): Express {
  const app = express();

  // Middleware
  app.use(express.json());

  // Health check endpoint
  app.get('/health', (_req: Request, res: Response) => {
    res.json({ status: 'ok' });
  });

  // API routes
  app.use('/api', tasksRouter);

  // Error handling middleware
  app.use((err: Error, _req: Request, res: Response, _next: NextFunction) => {
    console.error('Unhandled error:', err);
    res.status(500).json({
      error: 'Internal server error',
      message: 'An unexpected error occurred',
    });
  });

  return app;
}

/**
 * Start the server
 */
export function startServer(port: number = 3000): void {
  const app = createApp();

  app.listen(port, () => {
    console.log(`Agentium API server listening on port ${port}`);
  });
}

// Start server if this file is run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  const port = parseInt(process.env.PORT || '3000', 10);
  startServer(port);
}
