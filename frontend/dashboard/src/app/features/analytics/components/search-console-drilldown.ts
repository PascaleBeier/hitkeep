import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, input, signal } from '@angular/core';
import { takeUntilDestroyed, toSignal } from '@angular/core/rxjs-interop';
import { RouterLink } from '@angular/router';
import { forkJoin } from 'rxjs';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';
import { CardModule } from 'primeng/card';
import { MessageModule } from 'primeng/message';
import { TagModule } from 'primeng/tag';

import { MetricStat } from '@models/analytics.types';
import { GoogleSearchConsoleService, GoogleSearchConsoleSiteMapping, SearchConsoleDimensionRow, SearchConsoleMetricPoint, SearchConsoleOverview, SearchConsoleReportFilters } from '@services/google-search-console.service';
import { KpiCard } from './kpi-card';
import { MetricList } from './metric-list';
import { SeriesChart, SeriesChartPoint, SeriesDefinition } from './series-chart';

interface KpiCardData {
    label: string;
    value: string;
}

const SEARCH_CONSOLE_ALPHA3_TO_ALPHA2 = new Map(
    [
        'abw:AW',
        'afg:AF',
        'ago:AO',
        'aia:AI',
        'ala:AX',
        'alb:AL',
        'and:AD',
        'are:AE',
        'arg:AR',
        'arm:AM',
        'asm:AS',
        'ata:AQ',
        'atf:TF',
        'atg:AG',
        'aus:AU',
        'aut:AT',
        'aze:AZ',
        'bdi:BI',
        'bel:BE',
        'ben:BJ',
        'bes:BQ',
        'bfa:BF',
        'bgd:BD',
        'bgr:BG',
        'bhr:BH',
        'bhs:BS',
        'bih:BA',
        'blm:BL',
        'blr:BY',
        'blz:BZ',
        'bmu:BM',
        'bol:BO',
        'bra:BR',
        'brb:BB',
        'brn:BN',
        'btn:BT',
        'bvt:BV',
        'bwa:BW',
        'can:CA',
        'cck:CC',
        'caf:CF',
        'civ:CI',
        'cmr:CM',
        'cod:CD',
        'cog:CG',
        'cok:CK',
        'che:CH',
        'chl:CL',
        'chn:CN',
        'col:CO',
        'com:KM',
        'cpv:CV',
        'cri:CR',
        'cub:CU',
        'cuw:CW',
        'cxr:CX',
        'cym:KY',
        'cyp:CY',
        'cze:CZ',
        'deu:DE',
        'dji:DJ',
        'dma:DM',
        'dnk:DK',
        'dom:DO',
        'dza:DZ',
        'ecu:EC',
        'egy:EG',
        'eri:ER',
        'esh:EH',
        'esp:ES',
        'est:EE',
        'eth:ET',
        'fin:FI',
        'fji:FJ',
        'flk:FK',
        'fra:FR',
        'fro:FO',
        'fsm:FM',
        'gab:GA',
        'gbr:GB',
        'geo:GE',
        'ggy:GG',
        'gha:GH',
        'gib:GI',
        'gin:GN',
        'glp:GP',
        'gmb:GM',
        'gnb:GW',
        'gnq:GQ',
        'grc:GR',
        'grd:GD',
        'grl:GL',
        'gtm:GT',
        'guf:GF',
        'gum:GU',
        'guy:GY',
        'hkg:HK',
        'hmd:HM',
        'hnd:HN',
        'hrv:HR',
        'hti:HT',
        'hun:HU',
        'idn:ID',
        'imn:IM',
        'ind:IN',
        'iot:IO',
        'irl:IE',
        'irn:IR',
        'irq:IQ',
        'isl:IS',
        'isr:IL',
        'ita:IT',
        'jam:JM',
        'jey:JE',
        'jor:JO',
        'jpn:JP',
        'kaz:KZ',
        'ken:KE',
        'kgz:KG',
        'khm:KH',
        'kir:KI',
        'kna:KN',
        'kor:KR',
        'kwt:KW',
        'lao:LA',
        'lbn:LB',
        'lbr:LR',
        'lby:LY',
        'lca:LC',
        'lie:LI',
        'lka:LK',
        'lso:LS',
        'ltu:LT',
        'lux:LU',
        'lva:LV',
        'mac:MO',
        'maf:MF',
        'mar:MA',
        'mco:MC',
        'mda:MD',
        'mdg:MG',
        'mdv:MV',
        'mex:MX',
        'mhl:MH',
        'mkd:MK',
        'mli:ML',
        'mlt:MT',
        'mmr:MM',
        'mne:ME',
        'mng:MN',
        'mnp:MP',
        'moz:MZ',
        'mrt:MR',
        'msr:MS',
        'mtq:MQ',
        'mus:MU',
        'mwi:MW',
        'mys:MY',
        'myt:YT',
        'nam:NA',
        'ncl:NC',
        'ner:NE',
        'nfk:NF',
        'nga:NG',
        'nic:NI',
        'niu:NU',
        'nld:NL',
        'nor:NO',
        'npl:NP',
        'nru:NR',
        'nzl:NZ',
        'omn:OM',
        'pak:PK',
        'pan:PA',
        'pcn:PN',
        'per:PE',
        'phl:PH',
        'plw:PW',
        'png:PG',
        'pol:PL',
        'pri:PR',
        'prk:KP',
        'prt:PT',
        'pry:PY',
        'pse:PS',
        'pyf:PF',
        'qat:QA',
        'reu:RE',
        'rou:RO',
        'rwa:RW',
        'rus:RU',
        'sau:SA',
        'sdn:SD',
        'sen:SN',
        'sgp:SG',
        'sgs:GS',
        'shn:SH',
        'sjm:SJ',
        'slb:SB',
        'sle:SL',
        'slv:SV',
        'smr:SM',
        'som:SO',
        'spm:PM',
        'srb:RS',
        'ssd:SS',
        'stp:ST',
        'sur:SR',
        'svk:SK',
        'svn:SI',
        'swe:SE',
        'swz:SZ',
        'sxm:SX',
        'syc:SC',
        'syr:SY',
        'tca:TC',
        'tcd:TD',
        'tgo:TG',
        'tha:TH',
        'tjk:TJ',
        'tkl:TK',
        'tkm:TM',
        'tls:TL',
        'ton:TO',
        'tto:TT',
        'tun:TN',
        'tur:TR',
        'tuv:TV',
        'twn:TW',
        'tza:TZ',
        'uga:UG',
        'ukr:UA',
        'umi:UM',
        'ury:UY',
        'usa:US',
        'uzb:UZ',
        'vat:VA',
        'vct:VC',
        'ven:VE',
        'vgb:VG',
        'vir:VI',
        'vnm:VN',
        'vut:VU',
        'wlf:WF',
        'wsm:WS',
        'yem:YE',
        'zaf:ZA',
        'zmb:ZM',
        'zwe:ZW'
    ].map((entry) => {
        const [alpha3, alpha2] = entry.split(':');
        return [alpha3, alpha2] as const;
    })
);

@Component({
    selector: 'app-search-console-drilldown',
    imports: [RouterLink, ButtonModule, CardModule, MessageModule, TagModule, TranslocoPipe, KpiCard, MetricList, SeriesChart],
    templateUrl: './search-console-drilldown.html',
    styleUrl: './search-console-drilldown.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SearchConsoleDrilldown {
    private readonly service = inject(GoogleSearchConsoleService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    siteId = input<string | null>(null);
    siteDomain = input<string | null>(null);
    from = input<string | null>(null);
    to = input<string | null>(null);
    path = input<string | null>(null);
    country = input<string | null>(null);
    device = input<string | null>(null);
    shareMode = input<boolean>(false);
    refreshKey = input<number>(0);

    protected readonly loading = signal(false);
    protected readonly error = signal(false);
    protected readonly mapping = signal<GoogleSearchConsoleSiteMapping | null>(null);
    protected readonly overview = signal<SearchConsoleOverview | null>(null);
    protected readonly series = signal<SearchConsoleMetricPoint[]>([]);
    protected readonly queries = signal<SearchConsoleDimensionRow[]>([]);
    protected readonly pages = signal<SearchConsoleDimensionRow[]>([]);
    protected readonly countries = signal<SearchConsoleDimensionRow[]>([]);
    protected readonly devices = signal<SearchConsoleDimensionRow[]>([]);
    private mappingRequestID = 0;
    private reportRequestID = 0;

    protected readonly visible = computed(() => !this.shareMode() && this.mapping()?.mapped === true);
    protected readonly setupVisible = computed(() => !this.shareMode() && this.mapping()?.mapped === false);
    protected readonly setupCanManage = computed(() => this.mapping()?.can_manage === true);
    protected readonly pending = computed(() => {
        const state = this.mapping()?.sync_status?.state;
        return state === 'pending' || state === 'running';
    });
    protected readonly hasRows = computed(() => Boolean(this.series().length || this.queries().length || this.pages().length || this.countries().length || this.devices().length));
    protected readonly kpiCards = computed<KpiCardData[]>(() => {
        this.activeLanguage();
        const overview = this.overview();
        return [
            {
                label: this.transloco.translate('searchConsole.kpis.clicks'),
                value: overview ? this.formatNumber(overview.clicks, { maximumFractionDigits: 0 }) : '-'
            },
            {
                label: this.transloco.translate('searchConsole.kpis.impressions'),
                value: overview ? this.formatNumber(overview.impressions, { maximumFractionDigits: 0 }) : '-'
            },
            {
                label: this.transloco.translate('searchConsole.kpis.ctr'),
                value: overview ? `${this.formatNumber(overview.ctr * 100, { minimumFractionDigits: 1, maximumFractionDigits: 1 })}%` : '-'
            },
            {
                label: this.transloco.translate('searchConsole.kpis.position'),
                value: overview ? this.formatNumber(overview.average_position, { minimumFractionDigits: 1, maximumFractionDigits: 1 }) : '-'
            }
        ];
    });
    protected readonly chartData = computed<SeriesChartPoint[]>(() =>
        this.series().map((point) => ({
            time: `${point.date}T00:00:00Z`,
            clicks: point.clicks,
            impressions: point.impressions
        }))
    );
    protected readonly chartSeries = computed<SeriesDefinition[]>(() => {
        this.activeLanguage();
        return [
            {
                key: 'clicks',
                label: this.transloco.translate('searchConsole.kpis.clicks'),
                color: '#2563eb',
                gradientFrom: 'rgba(37, 99, 235, 0.2)',
                gradientTo: 'rgba(37, 99, 235, 0.02)'
            },
            {
                key: 'impressions',
                label: this.transloco.translate('searchConsole.kpis.impressions'),
                color: '#059669',
                gradientFrom: 'rgba(5, 150, 105, 0.18)',
                gradientTo: 'rgba(5, 150, 105, 0.02)'
            }
        ];
    });
    protected readonly queryMetrics = computed<MetricStat[]>(() => this.rowsToMetricStats(this.queries()));
    protected readonly pageMetrics = computed<MetricStat[]>(() => this.rowsToMetricStats(this.pages(), (value) => this.displayPagePath(value)));
    protected readonly countryMetrics = computed<MetricStat[]>(() => this.rowsToMetricStats(this.countries(), (value) => this.normalizeCountry(value)));
    protected readonly deviceMetrics = computed<MetricStat[]>(() => this.rowsToMetricStats(this.devices(), (value) => this.formatDevice(value)));
    protected readonly contextTags = computed(() => {
        this.activeLanguage();
        const tags: string[] = [];
        const dateRange = this.dateRangeLabel();
        if (dateRange) {
            tags.push(this.transloco.translate('searchConsole.context.range', { value: dateRange }));
        }
        const path = this.path();
        if (path) {
            tags.push(this.transloco.translate('common.filters.page', { value: path }));
        }
        const country = this.country();
        if (country) {
            tags.push(this.transloco.translate('common.filters.country', { value: this.countryDisplayName(this.normalizeCountry(country)) }));
        }
        const device = this.device();
        if (device) {
            tags.push(this.transloco.translate('common.filters.device', { value: this.formatDevice(device) }));
        }
        return tags;
    });

    constructor() {
        effect(() => {
            const siteID = this.siteId();
            const shareMode = this.shareMode();
            const requestID = ++this.mappingRequestID;

            this.clearMapping();
            if (shareMode || !siteID) {
                return;
            }

            this.service
                .getSiteMapping(siteID)
                .pipe(takeUntilDestroyed(this.destroyRef))
                .subscribe({
                    next: (mapping) => {
                        if (requestID !== this.mappingRequestID) return;
                        this.mapping.set(mapping);
                    },
                    error: () => {
                        if (requestID !== this.mappingRequestID) return;
                        this.error.set(true);
                    }
                });
        });

        effect(() => {
            const siteID = this.siteId();
            const from = this.from();
            const to = this.to();
            const shareMode = this.shareMode();
            const mapped = this.mapping()?.mapped === true;
            const filters = this.reportFilters();
            this.refreshKey();
            const requestID = ++this.reportRequestID;

            this.clearReports();
            if (shareMode || !siteID || !from || !to || !mapped) {
                return;
            }

            this.loading.set(true);
            this.loadReports(siteID, filters, requestID);
        });
    }

    private loadReports(siteID: string, filters: SearchConsoleReportFilters, requestID: number): void {
        forkJoin({
            overview: this.service.getOverview(siteID, filters),
            series: this.service.getSeries(siteID, filters),
            queries: this.service.getQueries(siteID, { ...filters, limit: 5 }),
            pages: this.service.getPages(siteID, { ...filters, limit: 5 }),
            countries: this.service.getBreakdown(siteID, 'country', { ...filters, limit: 5 }),
            devices: this.service.getBreakdown(siteID, 'device', { ...filters, limit: 5 })
        })
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (report) => {
                    if (requestID !== this.reportRequestID) return;
                    this.overview.set(report.overview);
                    this.series.set(report.series.series);
                    this.queries.set(report.queries.rows);
                    this.pages.set(report.pages.rows);
                    this.countries.set(report.countries.rows);
                    this.devices.set(report.devices.rows);
                    this.loading.set(false);
                },
                error: () => {
                    if (requestID !== this.reportRequestID) return;
                    this.error.set(true);
                    this.loading.set(false);
                }
            });
    }

    private reportFilters(): SearchConsoleReportFilters {
        return {
            from: this.from() ?? undefined,
            to: this.to() ?? undefined,
            path: this.path(),
            country: this.country(),
            device: this.device()
        };
    }

    private clearMapping(): void {
        this.reportRequestID++;
        this.mapping.set(null);
        this.clearReports();
    }

    private clearReports(): void {
        this.loading.set(false);
        this.error.set(false);
        this.overview.set(null);
        this.series.set([]);
        this.queries.set([]);
        this.pages.set([]);
        this.countries.set([]);
        this.devices.set([]);
    }

    private rowsToMetricStats(rows: SearchConsoleDimensionRow[], label: (value: string) => string = (value) => value): MetricStat[] {
        return rows.map((row) => ({ name: label(row.value), value: row.clicks }));
    }

    private displayPagePath(raw: string): string {
        const value = raw.trim();
        if (!value) return '/';
        try {
            const url = new URL(/^https?:\/\//i.test(value) ? value : `https://${value}`);
            const path = `${url.pathname || '/'}${url.search}`;
            return path || '/';
        } catch {
            return value.startsWith('/') ? value : `/${value}`;
        }
    }

    private normalizeCountry(raw: string): string {
        const value = raw.trim();
        if (/^[a-z]{3}$/i.test(value)) {
            return SEARCH_CONSOLE_ALPHA3_TO_ALPHA2.get(value.toLowerCase()) ?? value.toUpperCase();
        }
        return value.toUpperCase();
    }

    private formatDevice(raw: string): string {
        const value = raw.trim().toLowerCase();
        if (!value) return raw;
        return `${value.charAt(0).toUpperCase()}${value.slice(1)}`;
    }

    private dateRangeLabel(): string {
        const from = this.parseDate(this.from());
        const to = this.parseDate(this.to());
        if (!from || !to) return '';
        const language = this.activeLanguage();
        const sameYear = from.getUTCFullYear() === to.getUTCFullYear();
        const start = new Intl.DateTimeFormat(language, {
            month: 'short',
            day: 'numeric',
            ...(sameYear ? {} : { year: 'numeric' })
        }).format(from);
        const end = new Intl.DateTimeFormat(language, {
            month: 'short',
            day: 'numeric',
            year: 'numeric'
        }).format(to);
        return `${start} - ${end}`;
    }

    private parseDate(value: string | null): Date | null {
        if (!value) return null;
        const parsed = new Date(value);
        return Number.isNaN(parsed.getTime()) ? null : parsed;
    }

    private countryDisplayName(value: string): string {
        const code = value.trim().toUpperCase();
        if (!/^[A-Z]{2}$/.test(code)) return value;
        try {
            const displayNames = new Intl.DisplayNames([this.activeLanguage()], { type: 'region' });
            return displayNames.of(code) ?? value;
        } catch {
            return value;
        }
    }

    private formatNumber(value: number, options: Intl.NumberFormatOptions): string {
        return new Intl.NumberFormat(this.activeLanguage(), options).format(value);
    }
}
