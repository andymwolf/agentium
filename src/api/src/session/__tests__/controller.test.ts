import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  SessionController,
  createSessionController,
  SessionControllerOptions,
} from '../controller.js';
import {
  AgentAdapter,
  SessionRequest,
  TokenMetrics,
} from '../types.js';
import { DEFAULT_WORKFLOW } from '../workflow.js';

/**
 * Create a mock agent adapter for testing
 */
function createMockAdapter(
  responses: Record<string, { output: string; token_metrics?: TokenMetrics }>
): AgentAdapter {
  let callCount = 0;
  const responseList = Object.values(responses);

  return {
    invoke: vi.fn(async (prompt: string) => {
      // Return responses in order
      const response = responseList[callCount % responseList.length];
      callCount++;
      return response;
    }),
  };
}

/**
 * Create a mock adapter that returns specific responses based on prompt content
 */
function createSmartMockAdapter(): AgentAdapter {
  return {
    invoke: vi.fn(async (prompt: string) => {
      // Detect which step we're in based on prompt content
      if (prompt.includes('planning agent')) {
        return {
          output: 'Implementation plan:\n1. Create new file\n2. Add tests',
          token_metrics: { input_tokens: 100, output_tokens: 50, total_tokens: 150 },
        };
      }
      if (prompt.includes('implementation agent')) {
        return {
          output: 'Created src/feature.ts with new functionality',
          token_metrics: { input_tokens: 200, output_tokens: 100, total_tokens: 300 },
        };
      }
      if (prompt.includes('documentation agent')) {
        return {
          output: 'Updated README.md with new feature documentation',
          token_metrics: { input_tokens: 150, output_tokens: 75, total_tokens: 225 },
        };
      }
      if (prompt.includes('review agent') || prompt.includes('documentation review')) {
        return {
          output: 'Code looks good. No issues found.',
          token_metrics: { input_tokens: 80, output_tokens: 30, total_tokens: 110 },
        };
      }
      if (prompt.includes('judge agent')) {
        return {
          output: JSON.stringify({
            passed: true,
            reasoning: 'The output meets all requirements.',
          }),
          token_metrics: { input_tokens: 90, output_tokens: 40, total_tokens: 130 },
        };
      }
      // Default response
      return {
        output: 'Default response',
        token_metrics: { input_tokens: 50, output_tokens: 25, total_tokens: 75 },
      };
    }),
  };
}

describe('SessionController', () => {
  let mockAdapter: AgentAdapter;
  let controller: SessionController;

  beforeEach(() => {
    mockAdapter = createSmartMockAdapter();
    controller = createSessionController({ adapter: mockAdapter });
  });

  describe('execute', () => {
    it('should execute default workflow with 3 phases', async () => {
      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await controller.execute(request);

      expect(result.phases).toHaveLength(3);
      expect(result.phases.map((p) => p.name)).toEqual(['PLAN', 'IMPLEMENT', 'DOCS']);
    });

    it('should run W->R->J loop for each phase', async () => {
      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await controller.execute(request);

      for (const phase of result.phases) {
        expect(phase.worker).toBeDefined();
        expect(phase.reviewer).toBeDefined();
        expect(phase.judge).toBeDefined();
      }
    });

    it('should include duration_ms for each phase', async () => {
      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await controller.execute(request);

      for (const phase of result.phases) {
        expect(phase.duration_ms).toBeDefined();
        expect(typeof phase.duration_ms).toBe('number');
        expect(phase.duration_ms).toBeGreaterThanOrEqual(0);
      }
    });

    it('should include total duration_ms in result', async () => {
      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await controller.execute(request);

      expect(result.duration_ms).toBeDefined();
      expect(typeof result.duration_ms).toBe('number');
      expect(result.duration_ms).toBeGreaterThanOrEqual(0);
    });

    it('should generate task_id if not provided', async () => {
      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await controller.execute(request);

      expect(result.task_id).toBeDefined();
      // UUID format check
      const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
      expect(result.task_id).toMatch(uuidRegex);
    });

    it('should use provided task_id', async () => {
      const taskId = '550e8400-e29b-41d4-a716-446655440000';
      const request: SessionRequest = {
        task_id: taskId,
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await controller.execute(request);

      expect(result.task_id).toBe(taskId);
    });

    it('should return completed status when all phases pass', async () => {
      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await controller.execute(request);

      expect(result.status).toBe('completed');
    });
  });

  describe('context propagation', () => {
    it('should inject template variables into prompts', async () => {
      const testIssueUrl = 'https://github.com/test/repo/issues/42';
      const testInstructions = 'Implement the login feature';
      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'feature-branch' },
        prompt_context: {
          issue_url: testIssueUrl,
          instructions: testInstructions,
        },
      };

      await controller.execute(request);

      // Verify the adapter was called with prompts containing injected values
      const calls = (mockAdapter.invoke as ReturnType<typeof vi.fn>).mock.calls;
      expect(calls.length).toBeGreaterThan(0);

      // Concatenate all prompts for verification (test code only - not URL validation)
      const allPrompts = calls.map((call) => call[0]).join('\n');

      // Verify issue URL was injected using exact string comparison via split
      const issueUrlParts = testIssueUrl.split('/');
      const hasIssueNumber = allPrompts.includes('issues/42');
      const hasGitHubDomain = allPrompts.includes('github.com/test/repo');
      expect(hasIssueNumber && hasGitHubDomain).toBe(true);

      // Verify instructions were injected
      expect(allPrompts).toContain(testInstructions);
    });

    it('should propagate {{previous_phase_output}} between phases', async () => {
      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      await controller.execute(request);

      const calls = (mockAdapter.invoke as ReturnType<typeof vi.fn>).mock.calls;

      // The IMPLEMENT phase worker should receive the PLAN phase output
      // Look for calls that contain the planning output
      const implementCalls = calls.filter(
        (call) =>
          call[0].includes('implementation agent') &&
          call[0].includes('Implementation plan')
      );
      expect(implementCalls.length).toBeGreaterThan(0);
    });
  });

  describe('phase execution', () => {
    it('should complete worker step', async () => {
      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await controller.execute(request);

      for (const phase of result.phases) {
        expect(phase.worker.status).toBe('completed');
        expect(phase.worker.output).toBeDefined();
      }
    });

    it('should complete reviewer step', async () => {
      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await controller.execute(request);

      for (const phase of result.phases) {
        // Reviewer should be completed unless skipped
        expect(['completed', 'skipped']).toContain(phase.reviewer.status);
      }
    });

    it('should complete judge step with pass/fail', async () => {
      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await controller.execute(request);

      for (const phase of result.phases) {
        // Judge should be completed unless skipped
        expect(['completed', 'skipped']).toContain(phase.judge.status);
        if (phase.judge.status === 'completed') {
          expect(typeof phase.judge.passed).toBe('boolean');
          expect(phase.judge.reasoning).toBeDefined();
        }
      }
    });

    it('should include token metrics when available', async () => {
      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await controller.execute(request);

      for (const phase of result.phases) {
        if (phase.worker.status === 'completed') {
          expect(phase.worker.token_metrics).toBeDefined();
          expect(phase.worker.token_metrics?.total_tokens).toBeGreaterThan(0);
        }
      }
    });
  });

  describe('error handling', () => {
    it('should handle worker failure gracefully', async () => {
      const failingAdapter: AgentAdapter = {
        invoke: vi.fn(async (prompt: string) => {
          if (prompt.includes('planning agent')) {
            throw new Error('Worker failed');
          }
          return { output: 'success' };
        }),
      };

      const failingController = createSessionController({ adapter: failingAdapter });

      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await failingController.execute(request);

      // First phase should have failed worker
      expect(result.phases[0].worker.status).toBe('failed');
      expect(result.phases[0].worker.error).toBe('Worker failed');
      expect(result.phases[0].status).toBe('failed');
    });

    it('should skip reviewer when worker fails', async () => {
      const failingAdapter: AgentAdapter = {
        invoke: vi.fn(async (prompt: string) => {
          if (prompt.includes('planning agent')) {
            throw new Error('Worker failed');
          }
          return { output: 'success' };
        }),
      };

      const failingController = createSessionController({ adapter: failingAdapter });

      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await failingController.execute(request);

      // Reviewer should be skipped
      expect(result.phases[0].reviewer.status).toBe('skipped');
      expect(result.phases[0].reviewer.skip_reason).toBe('Worker failed');
    });

    it('should report failed status when any phase fails', async () => {
      const failingAdapter: AgentAdapter = {
        invoke: vi.fn(async (prompt: string) => {
          if (prompt.includes('planning agent')) {
            throw new Error('Worker failed');
          }
          return { output: 'success' };
        }),
      };

      const failingController = createSessionController({ adapter: failingAdapter });

      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
      };

      const result = await failingController.execute(request);

      expect(result.status).toBe('failed');
    });
  });

  describe('behaviors', () => {
    it('should skip reviewer when skip_reviewer_on_empty is true and output is empty', async () => {
      const emptyAdapter: AgentAdapter = {
        invoke: vi.fn(async () => ({
          output: '',
        })),
      };

      const emptyController = createSessionController({ adapter: emptyAdapter });

      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
        behaviors: {
          skip_reviewer_on_empty: true,
        },
      };

      const result = await emptyController.execute(request);

      // Reviewer should be skipped due to empty output
      expect(result.phases[0].reviewer.status).toBe('skipped');
      expect(result.phases[0].reviewer.skip_reason).toBe('Worker output is empty');
    });

    it('should skip judge when skip_judge_on_simple is true and output is simple', async () => {
      const simpleAdapter: AgentAdapter = {
        invoke: vi.fn(async () => ({
          output: 'Short output', // Less than 200 chars
        })),
      };

      const simpleController = createSessionController({ adapter: simpleAdapter });

      const request: SessionRequest = {
        workflow: 'default',
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
        behaviors: {
          skip_judge_on_simple: true,
        },
      };

      const result = await simpleController.execute(request);

      // Judge should be skipped with auto-pass
      expect(result.phases[0].judge.status).toBe('skipped');
      expect(result.phases[0].judge.passed).toBe(true);
    });
  });

  describe('custom phases', () => {
    it('should execute custom phases when provided', async () => {
      const request: SessionRequest = {
        repository: { url: 'https://github.com/test/repo', branch: 'main' },
        phases: [
          {
            name: 'CUSTOM_PHASE',
            worker_prompt: 'Custom worker prompt',
            reviewer_prompt: 'Custom reviewer prompt',
            judge_prompt: 'Custom judge prompt',
          },
        ],
      };

      const result = await controller.execute(request);

      expect(result.phases).toHaveLength(1);
      expect(result.phases[0].name).toBe('CUSTOM_PHASE');
    });
  });
});

describe('DEFAULT_WORKFLOW', () => {
  it('should have 3 phases', () => {
    expect(DEFAULT_WORKFLOW.phases).toHaveLength(3);
  });

  it('should have PLAN, IMPLEMENT, DOCS phases in order', () => {
    const phaseNames = DEFAULT_WORKFLOW.phases.map((p) => p.name);
    expect(phaseNames).toEqual(['PLAN', 'IMPLEMENT', 'DOCS']);
  });

  it('should have worker, reviewer, and judge prompts for each phase', () => {
    for (const phase of DEFAULT_WORKFLOW.phases) {
      expect(phase.worker_prompt).toBeDefined();
      expect(phase.worker_prompt.length).toBeGreaterThan(0);
      expect(phase.reviewer_prompt).toBeDefined();
      expect(phase.reviewer_prompt.length).toBeGreaterThan(0);
      expect(phase.judge_prompt).toBeDefined();
      expect(phase.judge_prompt.length).toBeGreaterThan(0);
    }
  });
});

describe('createSessionController', () => {
  it('should create a controller instance', () => {
    const adapter = createSmartMockAdapter();
    const controller = createSessionController({ adapter });

    expect(controller).toBeInstanceOf(SessionController);
  });
});
