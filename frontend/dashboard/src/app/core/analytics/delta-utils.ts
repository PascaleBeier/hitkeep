/** Percentage share of `numerator` in `denominator`, guarding divide-by-zero. */
export function safeRate(numerator: number, denominator: number): number {
    if (denominator === 0) return 0;
    return (numerator / denominator) * 100;
}

/** Percentage change against a previous period; `null` when there is no baseline. */
export function calcDelta(current: number, previous: number): number | null {
    if (previous === 0) return null;
    return ((current - previous) / previous) * 100;
}
