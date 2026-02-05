import type { Credentials } from './task-config.js';

/**
 * Adapter to required provider mapping.
 * Maps agent adapter names to the LLM provider they require credentials for.
 */
const ADAPTER_PROVIDER_MAP: Record<string, 'anthropic' | 'openai'> = {
  // Anthropic adapters
  'claude-code': 'anthropic',
  'claude-sonnet': 'anthropic',
  'claude-opus': 'anthropic',
  'claude-haiku': 'anthropic',
  // OpenAI adapters
  codex: 'openai',
  'gpt-4': 'openai',
  'gpt-4-turbo': 'openai',
  'gpt-4o': 'openai',
};

/**
 * Validation result for credential requirements.
 */
export interface CredentialValidationResult {
  valid: boolean;
  missingCredentials: string[];
  error?: string;
}

/**
 * Get the required provider for an adapter.
 * Returns undefined if the adapter doesn't require external credentials.
 */
export function getRequiredProvider(adapter: string): 'anthropic' | 'openai' | undefined {
  return ADAPTER_PROVIDER_MAP[adapter];
}

/**
 * Validate that required credentials are present for the given adapters.
 *
 * @param adapters - List of adapter names that will be used (from phase configurations)
 * @param credentials - Optional credentials object from the task request
 * @returns Validation result indicating if credentials are sufficient
 */
export function validateCredentials(
  adapters: string[],
  credentials?: Credentials
): CredentialValidationResult {
  const missingCredentials: string[] = [];

  // Get unique required providers
  const requiredProviders = new Set<string>();
  for (const adapter of adapters) {
    const provider = getRequiredProvider(adapter);
    if (provider) {
      requiredProviders.add(provider);
    }
  }

  // Check if credentials are provided for each required provider
  for (const provider of requiredProviders) {
    if (provider === 'anthropic') {
      if (!credentials?.anthropic?.access_token) {
        missingCredentials.push('anthropic');
      }
    } else if (provider === 'openai') {
      if (!credentials?.openai?.access_token) {
        missingCredentials.push('openai');
      }
    }
  }

  if (missingCredentials.length > 0) {
    return {
      valid: false,
      missingCredentials,
      error: `Missing required credentials for providers: ${missingCredentials.join(', ')}`,
    };
  }

  return {
    valid: true,
    missingCredentials: [],
  };
}

/**
 * Phase configuration with optional worker.
 */
interface PhaseConfig {
  name: string;
  max_iterations?: number;
  prompt?: string;
  worker?: {
    adapter: string;
  };
}

/**
 * Extract adapters from phase configurations.
 * Collects unique adapter names from phases that specify a worker.
 */
export function extractAdaptersFromPhases(phases?: PhaseConfig[]): string[] {
  if (!phases) {
    return [];
  }

  const adapters = new Set<string>();
  for (const phase of phases) {
    if (phase.worker?.adapter) {
      adapters.add(phase.worker.adapter);
    }
  }

  return Array.from(adapters);
}
