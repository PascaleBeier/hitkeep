import { Service, inject, signal } from '@angular/core';
import type { HitkeepChartDesign } from '@core/charts/hitkeep-chart-options';
import { PreferenceStorage } from '@services/preference-storage';

const CHART_DESIGN_STORAGE_KEY = 'hitkeep.chartDesign';
const CHART_DESIGNS: readonly HitkeepChartDesign[] = ['area', 'line', 'bar'];

/** Remembers the chart design (area / line / bars) across charts and sessions, like the report range. */
@Service()
export class ChartDesignPreferencesService {
    private readonly storage = inject(PreferenceStorage);
    private readonly designState = signal<HitkeepChartDesign | null>(this.readStoredDesign());

    /** The remembered design, or null when the user never picked one (charts fall back to their default). */
    readonly design = this.designState.asReadonly();

    setDesign(design: HitkeepChartDesign): void {
        this.designState.set(design);
        this.storage.write(CHART_DESIGN_STORAGE_KEY, design);
    }

    private readStoredDesign(): HitkeepChartDesign | null {
        const stored = this.storage.read<HitkeepChartDesign>(CHART_DESIGN_STORAGE_KEY);
        return stored !== null && CHART_DESIGNS.includes(stored) ? stored : null;
    }
}
