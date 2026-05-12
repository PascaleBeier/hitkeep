import { Opportunity, OpportunityStatus, OpportunityType } from '@services/opportunities.service';

export type OpportunityFilter = 'all' | OpportunityType;
export type StatusFilter = 'all' | Exclude<OpportunityStatus, 'dismissed'>;
export type TagSeverity = 'success' | 'info' | 'warn' | 'danger' | 'secondary' | 'contrast';

export interface OpportunityFilterItem<T extends string> {
    value: T;
    label: string;
    count: number;
    active: boolean;
}

export interface OpportunityEvidenceView {
    id: string;
    label: string;
    value: string;
    detail: string | null;
}

export interface OpportunityView {
    source: Opportunity;
    id: string;
    kind: OpportunityType;
    status: OpportunityStatus;
    title: string;
    summary: string;
    action: string;
    typeLabel: string;
    confidenceLabel: string;
    statusLabel: string;
    impactValue: string;
    impactLabel: string;
    routeLabel: string;
    routeIcon: string;
    icon: string;
    severity: TagSeverity;
    score: number;
    scoreWidth: string;
    evidence: OpportunityEvidenceView[];
}
