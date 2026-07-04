import type { Provider } from '@angular/core';
import { provideEchartsCore } from 'ngx-echarts';
import * as echarts from 'echarts/core';
import { BarChart, LineChart } from 'echarts/charts';
import { AriaComponent, GridComponent, LegendComponent, TooltipComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';

echarts.use([AriaComponent, BarChart, GridComponent, LegendComponent, LineChart, TooltipComponent, CanvasRenderer]);

export function provideHitkeepEcharts(): Provider {
    return provideEchartsCore({ echarts });
}
