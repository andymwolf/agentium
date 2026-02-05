import { z } from 'zod';

/**
 * OAuth token credential for a provider
 */
export const ProviderCredentialSchema = z.object({
  access_token: z.string().min(1),
  token_type: z.enum(['Bearer']).optional().default('Bearer'),
});

/**
 * Credentials for LLM providers
 */
export const CredentialsSchema = z.object({
  anthropic: ProviderCredentialSchema.optional(),
  openai: ProviderCredentialSchema.optional(),
});

/**
 * Worker configuration for a phase
 */
export const WorkerConfigSchema = z.object({
  adapter: z.string().min(1),
});

/**
 * Phase configuration for custom workflow phases
 */
export const PhaseConfigSchema = z.object({
  name: z.string().min(1),
  max_iterations: z.number().int().positive().optional(),
  prompt: z.string().optional(),
  worker: WorkerConfigSchema.optional(),
});

/**
 * Repository configuration
 */
export const RepositoryConfigSchema = z.object({
  url: z
    .string()
    .url()
    .refine(
      (url) => {
        try {
          const parsed = new URL(url);
          return parsed.hostname === 'github.com';
        } catch {
          return false;
        }
      },
      { message: 'Repository URL must be a valid GitHub URL' }
    ),
  branch: z.string().min(1),
});

/**
 * Prompt context configuration
 */
export const PromptContextSchema = z.object({
  issue_url: z.string().url().optional(),
  instructions: z.string().optional(),
});

/**
 * Behavior configuration
 */
export const BehaviorsSchema = z.object({
  skip_reviewer_on_empty: z.boolean().optional(),
  skip_judge_on_simple: z.boolean().optional(),
  judge_on_failure: z.boolean().optional(),
  auto_retry_on_rate_limit: z.boolean().optional(),
});

/**
 * Webhook configuration
 */
export const WebhookConfigSchema = z.object({
  url: z
    .string()
    .url()
    .refine(
      (url) => {
        try {
          const parsed = new URL(url);
          return parsed.protocol === 'https:';
        } catch {
          return false;
        }
      },
      { message: 'Webhook URL must use HTTPS' }
    ),
  secret: z.string().min(1),
});

/**
 * Task configuration request body schema
 */
export const TaskConfigSchema = z
  .object({
    task_id: z.string().uuid().optional(),
    repository: RepositoryConfigSchema,
    workflow: z.literal('default').optional(),
    phases: z.array(PhaseConfigSchema).optional(),
    prompt_context: PromptContextSchema.optional(),
    behaviors: BehaviorsSchema.optional(),
    webhook: WebhookConfigSchema.optional(),
    credentials: CredentialsSchema.optional(),
  })
  .refine(
    (data) => {
      // workflow and phases are mutually exclusive
      if (data.workflow !== undefined && data.phases !== undefined) {
        return false;
      }
      return true;
    },
    {
      message: 'Cannot specify both "workflow" and "phases" - they are mutually exclusive',
      path: ['workflow'],
    }
  );

/**
 * Task configuration type
 */
export type TaskConfig = z.infer<typeof TaskConfigSchema>;

/**
 * Provider credential type
 */
export type ProviderCredential = z.infer<typeof ProviderCredentialSchema>;

/**
 * Credentials type
 */
export type Credentials = z.infer<typeof CredentialsSchema>;

/**
 * Task accepted response
 */
export interface TaskAcceptedResponse {
  task_id: string;
  status: 'accepted';
  workflow?: 'default';
  estimated_start: string;
}

/**
 * Validation error response
 */
export interface ValidationErrorResponse {
  error: string;
  details?: z.ZodError['errors'];
}
