import { Router, Request, Response } from 'express';
import { v4 as uuidv4 } from 'uuid';
import { ZodError } from 'zod';
import {
  TaskConfigSchema,
  TaskConfig,
  TaskAcceptedResponse,
  ValidationErrorResponse,
} from '../schemas/task-config.js';
import { apiKeyAuth } from '../middleware/auth.js';

const router = Router();

/**
 * POST /v1/tasks
 *
 * Accept a new task configuration for execution.
 * Returns a task ID and acceptance status.
 */
router.post('/v1/tasks', apiKeyAuth, (req: Request, res: Response): void => {
  try {
    // Validate request body
    const validationResult = TaskConfigSchema.safeParse(req.body);

    if (!validationResult.success) {
      const errorResponse: ValidationErrorResponse = {
        error: 'Validation failed',
        details: validationResult.error.errors,
      };
      res.status(400).json(errorResponse);
      return;
    }

    const taskConfig: TaskConfig = validationResult.data;

    // Generate task ID if not provided
    const taskId = taskConfig.task_id || uuidv4();

    // Calculate estimated start time (for MVP: now + 5 seconds)
    const estimatedStart = new Date(Date.now() + 5000).toISOString();

    // Build response
    const response: TaskAcceptedResponse = {
      task_id: taskId,
      status: 'accepted',
      estimated_start: estimatedStart,
    };

    // Include workflow in response if specified
    if (taskConfig.workflow) {
      response.workflow = taskConfig.workflow;
    }

    res.status(200).json(response);
  } catch (error) {
    // Handle unexpected errors
    console.error('Error processing task request:', error);
    res.status(500).json({
      error: 'Internal server error',
      message: 'An unexpected error occurred while processing the request',
    });
  }
});

export { router as tasksRouter };
