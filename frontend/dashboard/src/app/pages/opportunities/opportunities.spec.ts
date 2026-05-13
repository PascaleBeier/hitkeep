import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { provideRouter } from '@angular/router';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { OpportunitiesPage } from './opportunities';
import { SiteService } from '@features/sites/services/site.service';
import { PermissionService, UserPermissions } from '@services/permission.service';
import { ShareService } from '@services/share.service';

describe('OpportunitiesPage', () => {
    let fixture: ComponentFixture<OpportunitiesPage>;
    let httpMock: HttpTestingController;
    let permissions: PermissionService;
    let shareService: ShareService;
    let transloco: TranslocoService;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                OpportunitiesPage,
                NoopAnimationsModule,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { opportunities: 'Opportunities' },
                            common: {
                                loadingSiteData: 'Loading site data...',
                                noSiteSelected: 'No site selected',
                                selectDateRange: 'Select date range',
                                actions: { apply: 'Apply', cancel: 'Cancel' },
                                timeRanges: {
                                    last24Hours: 'Last 24 hours',
                                    last7Days: 'Last 7 days',
                                    last30Days: 'Last 30 days',
                                    lastYear: 'Last year',
                                    customRange: 'Custom range',
                                    customShort: 'Custom',
                                    customRangeSummary: '{{start}} - {{end}}'
                                }
                            },
                            opportunities: {
                                header: {
                                    eyebrow: 'Traffic guidance',
                                    title: 'Evidence-backed recommendations for this site',
                                    subtitle: 'Ranked suggestions built from goals, funnels, ecommerce, search, and AI visibility.',
                                    scoreLabel: 'Score',
                                    scoreAria: 'Opportunity score',
                                    scoreHint: 'Strong'
                                },
                                kpis: {
                                    open: 'Open',
                                    score: 'Average score',
                                    highConfidence: 'High confidence',
                                    aiSearchWins: 'AI/Search wins'
                                },
                                filters: {
                                    ariaLabel: 'Opportunity filters',
                                    typeTitle: 'Type',
                                    statusTitle: 'Status',
                                    types: {
                                        all: 'All',
                                        conversion: 'Conversion',
                                        traffic: 'Traffic',
                                        ai: 'AI visibility',
                                        search: 'Search',
                                        setup: 'Setup'
                                    },
                                    statuses: {
                                        all: 'All',
                                        new: 'New',
                                        saved: 'Saved',
                                        done: 'Done',
                                        dismissed: 'Dismissed'
                                    }
                                },
                                confidence: { high: 'High confidence', medium: 'Medium confidence' },
                                actions: {
                                    generate: 'Refresh opportunities',
                                    generating: 'Refreshing',
                                    save: 'Save',
                                    inspect: 'Inspect',
                                    markDone: 'Mark done',
                                    dismiss: 'Dismiss'
                                },
                                errors: {
                                    load: 'Could not load opportunities.',
                                    generate: 'Could not refresh opportunities.',
                                    status: 'Could not update opportunity.'
                                },
                                inbox: {
                                    ariaLabel: 'Opportunity inbox',
                                    title: 'Opportunity inbox',
                                    subtitle: '{{count}} ranked suggestions',
                                    cloudBadge: 'Cloud-ready AI copy'
                                },
                                topActions: {
                                    ariaLabel: 'Top recommended actions',
                                    title: 'Top 3 actions',
                                    subtitle: 'Start with these ranked opportunities first.'
                                },
                                impact: {
                                    qualifiedVisits: 'qualified visits',
                                    conversionLift: 'conversion lift',
                                    searchClicks: 'search clicks',
                                    blindSpots: 'tracking blind spots',
                                    ai_touched_pages: 'AI-touched pages',
                                    pageviews_to_route: 'pageviews to route',
                                    tracked_conversion_events: 'tracked conversion events',
                                    conversion_events_to_measure: 'conversion events to measure',
                                    funnel_steps_to_measure: 'funnel steps to measure'
                                },
                                routes: {
                                    checkout: 'Checkout: {{path}}',
                                    path: '{{path}}',
                                    source: '{{source}}',
                                    event: 'Event: {{event_name}}',
                                    funnel: 'Funnel: {{start_path}}',
                                    tracker: '{{asset}}'
                                },
                                evidence: {
                                    checkout_starts: 'Checkout starts',
                                    orders: 'Orders',
                                    checkout_conversion_rate: 'Checkout conversion',
                                    checkout_conversion_rate_detail: 'The checkout rate is {{rate}} below target',
                                    ai_requests: 'AI crawler requests',
                                    ai_paths: 'Unique crawled paths',
                                    top_ai_path: 'Top AI path',
                                    ai_referrals: 'AI referral visits',
                                    ai_path_pageviews: 'Pageviews on AI-touched path',
                                    pageviews: 'Pageviews',
                                    sessions: 'Sessions',
                                    top_source: 'Top source',
                                    source_hits: 'Source visits',
                                    total_pageviews: 'Total pageviews',
                                    tracked_events: 'Tracked events',
                                    suggested_goal_event: 'Suggested goal event',
                                    suggested_goal_event_count: 'Observed event count',
                                    suggested_funnel_start: 'Suggested funnel start',
                                    suggested_funnel_start_pageviews: 'Start pageviews',
                                    suggested_funnel_conversion_event: 'Suggested conversion step',
                                    suggested_funnel_event_count: 'Observed conversion events'
                                },
                                catalog: {
                                    checkout_conversion: {
                                        title: 'Review checkout drop-off',
                                        summary: 'Checkout starts are converting at {{conversion_rate}} across {{checkout_starts}} starts.',
                                        action: 'Inspect checkout friction before adding more traffic.',
                                        digest: 'Checkout conversion is {{conversion_rate}}.'
                                    },
                                    ai_visibility: {
                                        title: 'Review AI crawler attention',
                                        summary: 'AI assistants requested {{requests}} pages; the strongest path is {{top_path}}.',
                                        action: 'Strengthen the crawled page with clearer answers and conversion context.',
                                        digest: 'AI crawlers focused on {{top_path}}.'
                                    },
                                    traffic_quality: {
                                        title: 'Review traffic from {{source}}',
                                        summary: '{{source}} produced {{source_hits}} visits out of {{total_pageviews}} pageviews in this window.',
                                        action: 'Review the landing paths and conversion setup for this source.',
                                        digest: 'Review traffic signals from {{source}}.'
                                    },
                                    setup_goal_suggestion: {
                                        title: 'Create a goal for {{event_name}}',
                                        summary: '{{event_name}} fired {{event_count}} times, but it is not saved as a goal yet.',
                                        action: 'Create an event goal with value {{goal_value}} so HitKeep can rank conversion opportunities around it.',
                                        digest: '{{event_name}} should become a goal.'
                                    },
                                    setup_funnel_suggestion: {
                                        title: 'Create a funnel from {{start_path}} to {{conversion_event}}',
                                        summary: '{{start_path}} has traffic and {{conversion_event}} fired {{event_count}} times, but those steps are not saved as a funnel yet.',
                                        action: 'Create a {{step_count}}-step funnel for {{funnel_steps}} so HitKeep can monitor drop-off before the conversion.',
                                        digest: '{{start_path}} to {{conversion_event}} should become a funnel.'
                                    },
                                    tracking_setup: {
                                        title: 'Collect enough signal for recommendations',
                                        summary: 'Add conversion events so opportunities can be ranked.',
                                        action: 'Verify the tracker and add one conversion event.',
                                        digest: 'Tracking needs more signal.'
                                    }
                                },
                                aiStatus: {
                                    disabled: 'AI disabled',
                                    disabledHint: 'Saved opportunities remain available.',
                                    selfHosted: 'Self-hosted AI: {{provider}} {{model}}',
                                    cloudManaged: 'Cloud-managed AI',
                                    notConfigured: 'AI not configured',
                                    notConfiguredHint: 'Configure an AI provider.',
                                    budgetExhaustedHint: 'AI refreshes are paused.',
                                    providerUnknown: 'provider unknown',
                                    modelUnknown: 'model unknown'
                                },
                                card: { priority: 'Priority' },
                                detail: {
                                    estimatedImpact: 'Estimated impact',
                                    priorityScore: 'Priority score',
                                    priorityHint: 'Evidence weighted',
                                    why: 'Why this matters',
                                    nextBestAction: 'Next best action',
                                    readOnly: 'You have read-only access.'
                                },
                                empty: {
                                    title: 'No opportunities match this view',
                                    body: 'Try a different type or status filter.'
                                },
                                noSiteDescription: 'Select a site to view opportunities.',
                                items: {
                                    mobileCheckout: {
                                        title: 'Fix the mobile checkout leak',
                                        summary: 'Mobile Safari checkout starts rose, but purchases are flat.',
                                        action: 'Review the mobile checkout step first.',
                                        evidence: ['Mobile conversion is down', 'Safari is the biggest segment', 'Pricing traffic is healthy'],
                                        plan: ['Replay the path', 'Shorten the form', 'Make payment errors visible']
                                    },
                                    aiPricing: {
                                        title: 'Review AI attention on pricing',
                                        summary: 'AI crawlers keep hitting pricing, but referral visits lag.',
                                        action: 'Add comparison copy above the pricing CTA.',
                                        evidence: ['Pricing has AI fetches', 'AI referrals convert well', 'No comparison section exists'],
                                        plan: ['Add AI-friendly answer copy', 'Add comparison schema', 'Link docs to pricing']
                                    },
                                    sourceAmplifier: {
                                        title: 'Review the source with clear engagement',
                                        summary: 'Google CPC has enough traffic to inspect landing paths.',
                                        action: 'Compare its landing paths against configured goals.',
                                        evidence: ['High traffic share', 'Tracked sessions exist', 'Goal setup is available'],
                                        plan: ['Review landing paths', 'Check goal coverage', 'Tag follow-up campaigns']
                                    },
                                    searchCtr: {
                                        title: 'Recover clicks from a high-impression comparison page',
                                        summary: 'The page ranks, but the snippet underperforms.',
                                        action: 'Rewrite the title around the buyer question.',
                                        evidence: ['Many impressions', 'Low CTR', 'Good conversion after click'],
                                        plan: ['Rewrite title', 'Add proof in intro', 'Submit for reindexing']
                                    },
                                    downloadTracking: {
                                        title: 'Enable download and outbound tracking',
                                        summary: 'Important buyer actions are still invisible.',
                                        action: 'Turn on automatic download and outbound tracking.',
                                        evidence: ['Docs downloads exist', 'No download events yet', 'Outbound clicks are unmeasured'],
                                        plan: ['Enable tracker option', 'Verify events', 'Create a download goal']
                                    }
                                }
                            }
                        },
                        de: {
                            nav: { opportunities: 'Chancen' },
                            common: {
                                loadingSiteData: 'Seitendaten werden geladen...',
                                noSiteSelected: 'Keine Website ausgewählt',
                                selectDateRange: 'Zeitraum auswählen',
                                actions: { apply: 'Anwenden', cancel: 'Abbrechen' },
                                timeRanges: {
                                    last24Hours: 'Letzte 24 Stunden',
                                    last7Days: 'Letzte 7 Tage',
                                    last30Days: 'Letzte 30 Tage',
                                    lastYear: 'Letztes Jahr',
                                    customRange: 'Eigener Zeitraum',
                                    customShort: 'Eigener',
                                    customRangeSummary: '{{start}} - {{end}}'
                                }
                            },
                            opportunities: {
                                header: {
                                    eyebrow: 'Traffichinweise',
                                    title: 'Evidenzbasierte Empfehlungen für diese Website',
                                    subtitle: 'Priorisierte Hinweise aus Zielen, Funnels, E-Commerce, Suche und KI-Sichtbarkeit.',
                                    scoreLabel: 'Score',
                                    scoreAria: 'Opportunity-Score',
                                    scoreHint: 'Stark'
                                },
                                kpis: {
                                    open: 'Offen',
                                    score: 'Durchschnittsscore',
                                    highConfidence: 'Hohe Sicherheit',
                                    aiSearchWins: 'KI-/Suchchancen'
                                },
                                filters: {
                                    ariaLabel: 'Opportunity-Filter',
                                    typeTitle: 'Typ',
                                    statusTitle: 'Status',
                                    types: {
                                        all: 'Alle',
                                        conversion: 'Conversion',
                                        traffic: 'Traffic',
                                        ai: 'KI-Sichtbarkeit',
                                        search: 'Suche',
                                        setup: 'Setup'
                                    },
                                    statuses: {
                                        all: 'Alle',
                                        new: 'Neu',
                                        saved: 'Gespeichert',
                                        done: 'Erledigt',
                                        dismissed: 'Verworfen'
                                    }
                                },
                                confidence: { high: 'Hohe Sicherheit', medium: 'Mittlere Sicherheit' },
                                actions: {
                                    generate: 'Chancen aktualisieren',
                                    generating: 'Aktualisieren',
                                    save: 'Speichern',
                                    inspect: 'Prüfen',
                                    markDone: 'Als erledigt markieren',
                                    dismiss: 'Verwerfen'
                                },
                                errors: {
                                    load: 'Chancen konnten nicht geladen werden.',
                                    generate: 'Chancen konnten nicht aktualisiert werden.',
                                    status: 'Chance konnte nicht aktualisiert werden.'
                                },
                                inbox: {
                                    ariaLabel: 'Chancen-Posteingang',
                                    title: 'Chancen-Posteingang',
                                    subtitle: '{{count}} priorisierte Hinweise'
                                },
                                topActions: {
                                    ariaLabel: 'Wichtigste empfohlene Aktionen',
                                    title: 'Top 3 Aktionen',
                                    subtitle: 'Beginne mit diesen priorisierten Chancen.'
                                },
                                impact: {
                                    ai_touched_pages: 'KI-berührte Seiten',
                                    pageviews_to_route: 'Seitenaufrufe zum Routen',
                                    tracked_conversion_events: 'erfasste Conversion-Ereignisse'
                                },
                                routes: {
                                    checkout: 'Checkout: {{path}}',
                                    path: '{{path}}',
                                    source: '{{source}}',
                                    tracker: '{{asset}}'
                                },
                                evidence: {
                                    checkout_starts: 'Checkout-Starts',
                                    orders: 'Bestellungen',
                                    checkout_conversion_rate: 'Checkout-Conversion',
                                    checkout_conversion_rate_detail: 'Die Checkout-Rate liegt {{rate}} unter dem Ziel',
                                    ai_requests: 'KI-Crawler-Anfragen',
                                    ai_paths: 'Einzigartige gecrawlte Pfade',
                                    top_ai_path: 'Wichtigster KI-Pfad',
                                    ai_referrals: 'KI-Referral-Besuche',
                                    ai_path_pageviews: 'Seitenaufrufe auf KI-berührtem Pfad',
                                    pageviews: 'Seitenaufrufe',
                                    sessions: 'Sitzungen',
                                    top_source: 'Wichtigste Quelle',
                                    source_hits: 'Quellenbesuche',
                                    total_pageviews: 'Gesamte Seitenaufrufe',
                                    tracked_events: 'Erfasste Ereignisse'
                                },
                                catalog: {
                                    checkout_conversion: {
                                        title: 'Checkout-Abbruch prüfen',
                                        summary: 'Checkout-Starts konvertieren mit {{conversion_rate}} bei {{checkout_starts}} Starts.',
                                        action: 'Prüfe Checkout-Reibung, bevor du mehr Traffic einkaufst.',
                                        digest: 'Checkout-Conversion liegt bei {{conversion_rate}}.'
                                    },
                                    ai_visibility: {
                                        title: 'KI-Crawler-Aufmerksamkeit prüfen',
                                        summary: 'KI-Assistenten haben {{requests}} Seiten angefragt; der stärkste Pfad ist {{top_path}}.',
                                        action: 'Stärke die gecrawlte Seite mit klareren Antworten und Conversion-Kontext.',
                                        digest: 'KI-Crawler konzentrieren sich auf {{top_path}}.'
                                    },
                                    traffic_quality: {
                                        title: 'Traffic von {{source}} prüfen',
                                        summary: '{{source}} erzeugte {{source_hits}} Besuche bei {{total_pageviews}} Seitenaufrufen in diesem Zeitraum.',
                                        action: 'Prüfe Landingpages und Conversion-Setup für diese Quelle.',
                                        digest: 'Prüfe Traffic-Signale von {{source}}.'
                                    },
                                    tracking_setup: {
                                        title: 'Genug Signal für Empfehlungen sammeln',
                                        summary: 'Füge Conversion-Ereignisse hinzu, damit Chancen priorisiert werden können.',
                                        action: 'Prüfe den Tracker und füge ein Conversion-Ereignis hinzu.',
                                        digest: 'Das Tracking braucht mehr Signal.'
                                    }
                                },
                                aiStatus: {
                                    disabled: 'KI deaktiviert',
                                    disabledHint: 'Gespeicherte Chancen bleiben sichtbar.',
                                    selfHosted: 'Selbst gehostete KI: {{provider}} {{model}}',
                                    cloudManaged: 'Cloud-verwaltete KI',
                                    notConfigured: 'KI nicht konfiguriert',
                                    notConfiguredHint: 'Konfiguriere einen KI-Anbieter.',
                                    budgetExhaustedHint: 'KI-Aktualisierungen pausieren.',
                                    providerUnknown: 'Anbieter unbekannt',
                                    modelUnknown: 'Modell unbekannt'
                                },
                                card: { priority: 'Priorität' },
                                detail: {
                                    estimatedImpact: 'Geschätzter Effekt',
                                    priorityScore: 'Prioritätsscore',
                                    priorityHint: 'Nach Evidenz gewichtet',
                                    why: 'Warum das wichtig ist',
                                    nextBestAction: 'Nächste beste Aktion',
                                    readOnly: 'Du hast nur Lesezugriff.'
                                },
                                empty: {
                                    title: 'Keine Chancen für diese Ansicht',
                                    body: 'Wähle einen anderen Typ oder Status.'
                                },
                                noSiteDescription: 'Wähle eine Website aus, um Chancen zu sehen.'
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en', 'de'],
                        defaultLang: 'en'
                    }
                })
            ],
            providers: [
                provideRouter([]),
                provideHttpClient(),
                provideHttpClientTesting(),
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: {
                        en: 'en-US',
                        'en-US': 'en-US'
                    }
                }),
                {
                    provide: SiteService,
                    useValue: {
                        activeSite: signal({ id: 'site-1', domain: 'example.com' }),
                        isLoading: signal(false)
                    }
                }
            ]
        }).compileComponents();

        httpMock = TestBed.inject(HttpTestingController);
        permissions = TestBed.inject(PermissionService);
        shareService = TestBed.inject(ShareService);
        shareService.clear();
        transloco = TestBed.inject(TranslocoService);
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('renders the evidence-backed opportunity inbox without money positioning', () => {
        renderDashboard();

        expect(fixture.nativeElement.textContent).toContain('Evidence-backed recommendations for this site');
        expect(fixture.nativeElement.textContent).toContain('Review checkout drop-off');
        expect(fixture.nativeElement.textContent).toContain('Checkout starts are converting at 42%');
        expect(fixture.nativeElement.textContent).not.toContain('API should not render me');
        expect(fixture.nativeElement.textContent).not.toContain('$8,500');
        expect(fixture.nativeElement.textContent).not.toContain('Upside');
    });

    it('renders the top three actionable opportunities before the full inbox', () => {
        renderDashboard({
            opportunities: [
                buildOpportunity({
                    id: 'done-high-score',
                    status: 'done',
                    score: 99,
                    type_key: 'opportunities.types.tracking_setup',
                    title_key: 'opportunities.catalog.tracking_setup.title',
                    summary_key: 'opportunities.catalog.tracking_setup.summary',
                    action_key: 'opportunities.catalog.tracking_setup.action',
                    digest_key: 'opportunities.catalog.tracking_setup.digest',
                    impact_label_key: 'opportunities.impact.tracked_conversion_events'
                }),
                buildOpportunity({
                    id: 'top-ai',
                    kind: 'ai',
                    status: 'saved',
                    score: 92,
                    type_key: 'opportunities.types.ai_visibility',
                    title_key: 'opportunities.catalog.ai_visibility.title',
                    summary_key: 'opportunities.catalog.ai_visibility.summary',
                    action_key: 'opportunities.catalog.ai_visibility.action',
                    digest_key: 'opportunities.catalog.ai_visibility.digest',
                    copy_params: { requests: 420, top_path: '/pricing' },
                    impact_value: '+420',
                    impact_label_key: 'opportunities.impact.ai_touched_pages'
                }),
                buildOpportunity({
                    id: 'top-checkout',
                    score: 88,
                    copy_params: { conversion_rate: '42%', checkout_starts: 500 },
                    impact_value: '500'
                }),
                buildOpportunity({
                    id: 'top-source',
                    kind: 'traffic',
                    score: 74,
                    type_key: 'opportunities.types.traffic_quality',
                    title_key: 'opportunities.catalog.traffic_quality.title',
                    summary_key: 'opportunities.catalog.traffic_quality.summary',
                    action_key: 'opportunities.catalog.traffic_quality.action',
                    digest_key: 'opportunities.catalog.traffic_quality.digest',
                    copy_params: { source: 'Open Alternative', source_hits: 240, total_pageviews: 1200 }
                }),
                buildOpportunity({
                    id: 'dismissed-setup',
                    status: 'dismissed',
                    type_key: 'opportunities.types.tracking_setup',
                    title_key: 'opportunities.catalog.tracking_setup.title',
                    summary_key: 'opportunities.catalog.tracking_setup.summary',
                    action_key: 'opportunities.catalog.tracking_setup.action',
                    digest_key: 'opportunities.catalog.tracking_setup.digest',
                    impact_label_key: 'opportunities.impact.tracked_conversion_events'
                })
            ]
        });

        const topActions = fixture.nativeElement.querySelector('.opportunities-top-actions') as HTMLElement | null;
        expect(topActions?.textContent).toContain('Top 3 actions');
        expect(topActions?.querySelectorAll('app-opportunity-card').length).toBe(3);
        expect(topActions?.textContent).toContain('Review AI crawler attention');
        expect(topActions?.textContent).toContain('Review checkout drop-off');
        expect(topActions?.textContent).toContain('Review traffic from Open Alternative');
        expect(topActions?.textContent).not.toContain('Collect enough signal for recommendations');
    });

    it('does not render the email digest preview on the decision page', () => {
        renderDashboard();

        expect(fixture.nativeElement.querySelector('.opportunities-digest')).toBeNull();
        httpMock.expectNone('/api/sites/site-1/opportunities/digest-preview?frequency=weekly');
    });

    it('does not render the digest preview in share mode', () => {
        renderDashboard({
            shareToken: 'share-token',
            permissions: {
                instance_role: 'user',
                permissions: {
                    'site-1': 'viewer'
                }
            }
        });

        expect(fixture.nativeElement.querySelector('.opportunities-digest')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Review checkout drop-off');
    });

    it('renders average score instead of aggregate currency value', () => {
        renderDashboard({
            opportunities: [
                buildOpportunity({
                    id: 'score-a',
                    score: 90,
                    copy_params: { conversion_rate: '42%', checkout_starts: 500 },
                    impact_value: '500'
                }),
                buildOpportunity({
                    id: 'score-b',
                    score: 70,
                    copy_params: { conversion_rate: '35%', checkout_starts: 300 },
                    impact_value: '300'
                })
            ]
        });

        expect(fixture.nativeElement.textContent).toContain('Average score');
        expect(fixture.nativeElement.textContent).toContain('80');
        expect(fixture.nativeElement.textContent).not.toContain('Estimated monthly');
    });

    it('filters opportunities by AI visibility', () => {
        renderDashboard();

        const buttons = Array.from(fixture.nativeElement.querySelectorAll('button')) as HTMLButtonElement[];
        buttons.find((button) => button.textContent?.includes('AI visibility'))?.click();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Review AI crawler attention');
        expect(fixture.nativeElement.textContent).not.toContain('Review checkout drop-off');
    });

    it('regenerates opportunities for the selected range', () => {
        renderDashboard();

        const buttons = Array.from(fixture.nativeElement.querySelectorAll('button')) as HTMLButtonElement[];
        buttons.find((button) => button.textContent?.includes('Refresh opportunities'))?.click();

        const req = httpMock.expectOne((request) => request.method === 'POST' && request.url === '/api/sites/site-1/opportunities/generate');
        expect(req.request.params.has('from')).toBe(true);
        expect(req.request.params.has('to')).toBe(true);
        req.flush({
            opportunities: [
                buildOpportunity({
                    id: 'op-3',
                    kind: 'traffic',
                    type_key: 'opportunities.types.traffic_quality',
                    title_key: 'opportunities.catalog.traffic_quality.title',
                    summary_key: 'opportunities.catalog.traffic_quality.summary',
                    action_key: 'opportunities.catalog.traffic_quality.action',
                    digest_key: 'opportunities.catalog.traffic_quality.digest',
                    copy_params: { source: 'Google', source_hits: '1,200', total_pageviews: '4,000' },
                    impact_value: '1,200'
                })
            ],
            ai_status: 'success'
        });
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Review traffic from Google');
        expect(fixture.nativeElement.textContent).toContain('1,200');
    });

    it('keeps the inbox stable when generation returns nullable opportunities', () => {
        renderDashboard();

        const buttons = Array.from(fixture.nativeElement.querySelectorAll('button')) as HTMLButtonElement[];
        buttons.find((button) => button.textContent?.includes('Refresh opportunities'))?.click();

        const req = httpMock.expectOne((request) => request.method === 'POST' && request.url === '/api/sites/site-1/opportunities/generate');
        req.flush({
            opportunities: null,
            ai_status: 'success'
        });
        expect(() => fixture.detectChanges()).not.toThrow();
        expect(fixture.nativeElement.textContent).toContain('No opportunities match this view');
    });

    it('keeps the inbox stable when generation returns nullable nested opportunity fields', () => {
        renderDashboard();

        const buttons = Array.from(fixture.nativeElement.querySelectorAll('button')) as HTMLButtonElement[];
        buttons.find((button) => button.textContent?.includes('Refresh opportunities'))?.click();

        const req = httpMock.expectOne((request) => request.method === 'POST' && request.url === '/api/sites/site-1/opportunities/generate');
        req.flush({
            opportunities: [
                buildOpportunity({
                    id: 'op-null-nested',
                    copy_params: null,
                    route_params: null,
                    evidence: null,
                    cited_evidence_ids: null
                })
            ],
            ai_status: 'success'
        });
        expect(() => fixture.detectChanges()).not.toThrow();
        expect(fixture.nativeElement.textContent).toContain('Review checkout drop-off');
    });

    it('lets site viewers read localized opportunities without mutation actions', () => {
        renderDashboard({
            permissions: {
                instance_role: 'user',
                permissions: {
                    'site-1': 'viewer'
                }
            }
        });

        expect(fixture.nativeElement.textContent).toContain('Review checkout drop-off');
        expect(fixture.nativeElement.textContent).not.toContain('Refresh opportunities');
        expect(fixture.nativeElement.textContent).not.toContain('AI disabled');

        const buttons = Array.from(fixture.nativeElement.querySelectorAll('button')) as HTMLButtonElement[];
        expect(buttons.some((button) => button.textContent?.trim() === 'Save')).toBe(false);
        buttons.find((button) => button.textContent?.includes('Inspect'))?.click();
        fixture.detectChanges();

        expect(document.body.textContent).toContain('Inspect checkout friction before adding more traffic.');
        expect(document.body.textContent).toContain('You have read-only access.');
        expect(document.body.textContent).not.toContain('Mark done');
        expect(document.body.textContent).not.toContain('Dismiss');
    });

    it('renders disabled AI status without secrets', () => {
        renderDashboard({ aiStatus: { status: 'disabled', enabled: false, configured: false } });
        expect(fixture.nativeElement.textContent).toContain('AI disabled');
        expect(fixture.nativeElement.textContent).toContain('Saved opportunities remain available.');
        expect(fixture.nativeElement.textContent).not.toContain('sk-');
    });

    it('removes no-op save actions after an opportunity is saved', () => {
        renderDashboard({ opportunities: [buildOpportunity({ id: 'op-save' })] });

        const saveButton = (Array.from(fixture.nativeElement.querySelectorAll('button')) as HTMLButtonElement[]).find((button) => button.textContent?.trim() === 'Save');
        expect(saveButton).toBeTruthy();
        saveButton?.click();

        const req = httpMock.expectOne('/api/sites/site-1/opportunities/op-save');
        expect(req.request.method).toBe('PATCH');
        expect(req.request.body).toEqual({ status: 'saved' });
        req.flush(buildOpportunity({ id: 'op-save', status: 'saved' }));
        fixture.detectChanges();

        const buttons = Array.from(fixture.nativeElement.querySelectorAll('button')) as HTMLButtonElement[];
        expect(buttons.some((button) => button.textContent?.trim() === 'Save')).toBe(false);
    });

    it('renders cloud-managed AI status without provider secrets', () => {
        renderDashboard({ aiStatus: { status: 'configured', enabled: true, configured: true, config_mode: 'cloud_managed', provider: 'bedrock', model: 'claude-test' } });
        expect(fixture.nativeElement.textContent).toContain('Cloud-managed AI');
        expect(fixture.nativeElement.textContent).not.toContain('sk-');
    });

    it('renders the same opportunity from German key translations', () => {
        renderDashboard();
        transloco.setActiveLang('de');
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Checkout-Abbruch prüfen');
        expect(fixture.nativeElement.textContent).toContain('Checkout-Starts konvertieren mit 42%');
        expect(fixture.nativeElement.textContent).not.toContain('Review checkout drop-off');
        expect(fixture.nativeElement.textContent).not.toContain('API should not render me');
    });

    it('renders evidence detail copy from translation keys and params', () => {
        renderDashboard({
            opportunities: [
                buildOpportunity({
                    evidence: [
                        {
                            id: 'conversion_rate',
                            label_key: 'opportunities.evidence.checkout_conversion_rate',
                            value: '42%',
                            detail_key: 'opportunities.evidence.checkout_conversion_rate_detail',
                            detail_params: { rate: '13%' },
                            detail: 'API should not render this evidence detail'
                        }
                    ]
                })
            ]
        });

        expect(fixture.nativeElement.textContent).toContain('The checkout rate is 13% below target');
        expect(fixture.nativeElement.textContent).not.toContain('API should not render this evidence detail');
    });

    it('renders multi-signal AI visibility evidence from translation keys', () => {
        renderDashboard({
            opportunities: [
                buildOpportunity({
                    id: 'ai-multisignal',
                    kind: 'ai',
                    type_key: 'opportunities.types.ai_visibility',
                    title_key: 'opportunities.catalog.ai_visibility.title',
                    summary_key: 'opportunities.catalog.ai_visibility.summary',
                    action_key: 'opportunities.catalog.ai_visibility.action',
                    digest_key: 'opportunities.catalog.ai_visibility.digest',
                    copy_params: {
                        requests: 82,
                        top_path: '/pricing',
                        ai_referrals: 32,
                        top_path_pageviews: 420
                    },
                    impact_value: '+7',
                    impact_label_key: 'opportunities.impact.ai_touched_pages',
                    evidence: [
                        { id: 'ai_requests', label_key: 'opportunities.evidence.ai_requests', value: '82' },
                        { id: 'ai_referrals', label_key: 'opportunities.evidence.ai_referrals', value: '32' },
                        { id: 'ai_path_pageviews', label_key: 'opportunities.evidence.ai_path_pageviews', value: '420' }
                    ],
                    cited_evidence_ids: ['ai_requests', 'ai_referrals', 'ai_path_pageviews']
                })
            ]
        });

        expect(fixture.nativeElement.textContent).toContain('AI referral visits: 32');
        expect(fixture.nativeElement.textContent).toContain('Pageviews on AI-touched path: 420');
        expect(fixture.nativeElement.textContent).not.toContain('opportunities.evidence.ai_referrals');
    });

    it('renders setup goal suggestions from translation keys and safe placeholders', () => {
        renderDashboard({
            opportunities: [
                buildOpportunity({
                    id: 'goal-setup',
                    kind: 'setup',
                    type_key: 'opportunities.types.setup_goal_suggestion',
                    title_key: 'opportunities.catalog.setup_goal_suggestion.title',
                    summary_key: 'opportunities.catalog.setup_goal_suggestion.summary',
                    action_key: 'opportunities.catalog.setup_goal_suggestion.action',
                    digest_key: 'opportunities.catalog.setup_goal_suggestion.digest',
                    copy_params: {
                        event_name: 'demo_request',
                        event_count: 18,
                        goal_value: 'demo_request'
                    },
                    impact_value: '18',
                    impact_label_key: 'opportunities.impact.conversion_events_to_measure',
                    route_label_key: 'opportunities.routes.event',
                    route_params: { event_name: 'demo_request' },
                    route_icon: 'pi pi-bullseye',
                    evidence: [
                        { id: 'suggested_goal_event', label_key: 'opportunities.evidence.suggested_goal_event', value: 'demo_request' },
                        { id: 'suggested_goal_event_count', label_key: 'opportunities.evidence.suggested_goal_event_count', value: '18' }
                    ],
                    cited_evidence_ids: ['suggested_goal_event', 'suggested_goal_event_count'],
                    title: 'API should not render me',
                    summary: 'API should not render me'
                })
            ]
        });

        expect(fixture.nativeElement.textContent).toContain('Create a goal for demo_request');
        expect(fixture.nativeElement.textContent).toContain('demo_request fired 18 times');
        expect(fixture.nativeElement.textContent).toContain('conversion events to measure');
        expect(fixture.nativeElement.textContent).toContain('Observed event count: 18');
        expect(fixture.nativeElement.textContent).toContain('Event: demo_request');
        expect(fixture.nativeElement.textContent).not.toContain('API should not render me');
    });

    it('renders setup funnel suggestions from translation keys and safe placeholders', () => {
        renderDashboard({
            opportunities: [
                buildOpportunity({
                    id: 'funnel-setup',
                    kind: 'setup',
                    type_key: 'opportunities.types.setup_funnel_suggestion',
                    title_key: 'opportunities.catalog.setup_funnel_suggestion.title',
                    summary_key: 'opportunities.catalog.setup_funnel_suggestion.summary',
                    action_key: 'opportunities.catalog.setup_funnel_suggestion.action',
                    digest_key: 'opportunities.catalog.setup_funnel_suggestion.digest',
                    copy_params: {
                        start_path: '/pricing',
                        conversion_event: 'demo_request',
                        event_count: 18,
                        step_count: 2,
                        funnel_steps: '/pricing -> demo_request'
                    },
                    impact_value: '2',
                    impact_label_key: 'opportunities.impact.funnel_steps_to_measure',
                    route_label_key: 'opportunities.routes.funnel',
                    route_params: { start_path: '/pricing' },
                    route_icon: 'pi pi-sitemap',
                    evidence: [
                        { id: 'suggested_funnel_start', label_key: 'opportunities.evidence.suggested_funnel_start', value: '/pricing' },
                        { id: 'suggested_funnel_event_count', label_key: 'opportunities.evidence.suggested_funnel_event_count', value: '18' }
                    ],
                    cited_evidence_ids: ['suggested_funnel_start', 'suggested_funnel_event_count'],
                    title: 'API should not render me',
                    summary: 'API should not render me'
                })
            ]
        });

        expect(fixture.nativeElement.textContent).toContain('Create a funnel from /pricing to demo_request');
        expect(fixture.nativeElement.textContent).toContain('/pricing has traffic and demo_request fired 18 times');
        expect(fixture.nativeElement.textContent).toContain('funnel steps to measure');
        expect(fixture.nativeElement.textContent).toContain('Observed conversion events: 18');
        expect(fixture.nativeElement.textContent).toContain('Funnel: /pricing');
        expect(fixture.nativeElement.textContent).not.toContain('API should not render me');
    });

    function renderDashboard(options: { permissions?: UserPermissions; aiStatus?: Partial<Record<string, unknown>>; opportunities?: unknown[]; shareToken?: string } = {}) {
        shareService.setToken(options.shareToken ?? null);
        permissions.applyPermissions(
            options.permissions ?? {
                instance_role: 'owner',
                permissions: {
                    'site-1': 'owner'
                }
            }
        );

        fixture = TestBed.createComponent(OpportunitiesPage);
        fixture.detectChanges();
        if (permissions.isInstanceAdmin()) {
            flushAIStatus(options.aiStatus);
        }
        flushList(options.opportunities, options.shareToken);
        fixture.detectChanges();
    }
});

function flushAIStatus(overrides: Partial<Record<string, unknown>> = {}) {
    const httpMock = TestBed.inject(HttpTestingController);
    httpMock.expectOne('/api/admin/system/ai').flush({
        status: 'configured',
        enabled: true,
        configured: true,
        config_mode: 'self_hosted',
        provider: 'openai',
        model: 'gpt-test',
        base_url_configured: false,
        requests_used: 0,
        request_limit: 100,
        tokens_used: 0,
        token_limit: 10000,
        budget_window_minutes: 60,
        budget_exhausted: false,
        ...overrides
    });
}

function flushList(opportunities?: unknown[], shareToken?: string) {
    const httpMock = TestBed.inject(HttpTestingController);
    const url = shareToken ? `/api/share/${shareToken}/sites/site-1/opportunities` : '/api/sites/site-1/opportunities';
    httpMock.expectOne(url).flush({
        opportunities: opportunities ?? [
            buildOpportunity({
                id: 'op-1',
                kind: 'conversion',
                type_key: 'opportunities.types.checkout_conversion',
                title_key: 'opportunities.catalog.checkout_conversion.title',
                summary_key: 'opportunities.catalog.checkout_conversion.summary',
                action_key: 'opportunities.catalog.checkout_conversion.action',
                digest_key: 'opportunities.catalog.checkout_conversion.digest',
                copy_params: {
                    conversion_rate: '42%',
                    checkout_starts: 500
                },
                impact_value: '500',
                impact_label_key: 'opportunities.impact.checkout_starts',
                evidence: [
                    { id: 'checkout_starts', label_key: 'opportunities.evidence.checkout_starts', value: '120' },
                    { id: 'conversion_rate', label_key: 'opportunities.evidence.checkout_conversion_rate', value: '42%' }
                ],
                cited_evidence_ids: ['checkout_starts', 'conversion_rate'],
                title: 'API should not render me',
                summary: 'API should not render me'
            }),
            buildOpportunity({
                id: 'op-2',
                kind: 'ai',
                type_key: 'opportunities.types.ai_visibility',
                title_key: 'opportunities.catalog.ai_visibility.title',
                summary_key: 'opportunities.catalog.ai_visibility.summary',
                action_key: 'opportunities.catalog.ai_visibility.action',
                digest_key: 'opportunities.catalog.ai_visibility.digest',
                copy_params: {
                    requests: 420,
                    top_path: '/pricing'
                },
                impact_value: '+420',
                impact_label_key: 'opportunities.impact.ai_touched_pages',
                evidence: [{ id: 'ai_requests', label_key: 'opportunities.evidence.ai_requests', value: '420' }],
                cited_evidence_ids: ['ai_requests']
            })
        ]
    });
}

function buildOpportunity(overrides: Partial<Record<string, unknown>> = {}) {
    return {
        id: 'op-default',
        site_id: 'site-1',
        kind: 'conversion',
        type_key: 'opportunities.types.checkout_conversion',
        title_key: 'opportunities.catalog.checkout_conversion.title',
        summary_key: 'opportunities.catalog.checkout_conversion.summary',
        action_key: 'opportunities.catalog.checkout_conversion.action',
        digest_key: 'opportunities.catalog.checkout_conversion.digest',
        copy_params: { conversion_rate: '42%', checkout_starts: 100 },
        impact_value: '100',
        impact_label_key: 'opportunities.impact.checkout_starts',
        confidence: 'medium',
        score: 82,
        score_breakdown: {
            sample: 82,
            impact: 70,
            urgency: 55,
            effort: 70,
            actionability: 85,
            evidence_fit: 99,
            freshness: 50,
            total: 82
        },
        status: 'new',
        route_label_key: 'opportunities.routes.checkout',
        route_params: { path: '/checkout', source: 'google', asset: 'hk.js' },
        route_icon: 'pi pi-arrow-right',
        detector_version: 'opportunities-detectors-v1',
        evidence: [{ id: 'conversion_rate', label_key: 'opportunities.evidence.checkout_conversion_rate', value: '42%' }],
        cited_evidence_ids: ['conversion_rate'],
        generated_at: '2026-05-09T10:00:00Z',
        created_at: '2026-05-09T10:00:00Z',
        updated_at: '2026-05-09T10:00:00Z',
        ...overrides
    };
}
