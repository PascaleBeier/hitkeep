/** One value per dimension; selecting the active value toggles it off. */
export function toggleDimensionFilter<T extends string>(filters: { type: T; value: string }[], type: T, value: string): { type: T; value: string }[] {
    if (!value) return filters;
    const index = filters.findIndex((filter) => filter.type === type);
    if (index < 0) return [...filters, { type, value }];
    if (filters[index].value === value) return filters.filter((_, i) => i !== index);
    const next = [...filters];
    next[index] = { type, value };
    return next;
}
