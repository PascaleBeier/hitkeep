import type { BillingInterval, CloudPlanCode } from '@services/cloud.service';

export interface CloudPurchaseIntent {
    plan: CloudPlanCode;
    billing: BillingInterval;
}

export function cloudPurchaseIntent(plan: string | null | undefined, billing: string | null | undefined): CloudPurchaseIntent {
    const normalizedPlan = plan?.trim().toLowerCase();
    const selectedPlan: CloudPlanCode = normalizedPlan === 'pro' || normalizedPlan === 'business' ? normalizedPlan : 'free';
    return {
        plan: selectedPlan,
        billing: selectedPlan === 'free' ? 'monthly' : billing?.trim().toLowerCase() === 'annual' ? 'annual' : 'monthly'
    };
}

export function cloudPurchaseQuery(intent: CloudPurchaseIntent): { plan: CloudPlanCode; billing: BillingInterval } {
    return { plan: intent.plan, billing: intent.billing };
}

export function cloudBillingReviewUrl(intent: CloudPurchaseIntent): string {
    const query = new URLSearchParams({ purchase: 'review', plan: intent.plan, billing: intent.billing });
    return `/admin/team?${query}`;
}
