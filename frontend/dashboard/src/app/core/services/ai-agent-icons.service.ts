import { Injectable, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { AIAgentCatalog } from '@models/analytics.types';

/**
 * Resolves favicon-lookup hosts for AI agents and AI referrer surfaces from
 * the embedded master-list catalog (`GET /api/ai-agents`). Icons are a
 * progressive enhancement: lookups return null until the catalog has loaded
 * and for entries without a documentation URL.
 */
@Injectable({ providedIn: 'root' })
export class AIAgentIconsService {
    private readonly http = inject(HttpClient);
    private requested = false;
    private readonly agentHosts = signal<Record<string, string>>({});
    private readonly referrerHosts = signal<Record<string, string>>({});

    agentIconHost(name: string): string | null {
        this.ensureLoaded();
        return this.agentHosts()[name] ?? null;
    }

    referrerIconHost(name: string): string | null {
        this.ensureLoaded();
        return this.referrerHosts()[name] ?? null;
    }

    private ensureLoaded(): void {
        if (this.requested) return;
        this.requested = true;
        this.http.get<AIAgentCatalog>('/api/ai-agents').subscribe({
            next: (catalog) => {
                const agents: Record<string, string> = {};
                for (const agent of catalog.agents ?? []) {
                    if (agent.icon_host) agents[agent.name] = agent.icon_host;
                }
                const referrers: Record<string, string> = {};
                for (const referrer of catalog.ai_referrers ?? []) {
                    if (referrer.icon_host) referrers[referrer.name] = referrer.icon_host;
                }
                this.agentHosts.set(agents);
                this.referrerHosts.set(referrers);
            },
            error: () => {
                // Icons are decorative; retry on the next lookup session.
                this.requested = false;
            }
        });
    }
}
