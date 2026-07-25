import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { MetricCardGroup } from './metric-card-group';

describe('MetricCardGroup', () => {
    let fixture: ComponentFixture<MetricCardGroup>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                MetricCardGroup,
                TranslocoTestingModule.forRoot({
                    langs: { en: { aiAgents: { provenance: { hint: 'tracked {{tracked}} · logs {{fetched}}' } } } },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: {
                        en: 'en-US',
                        'en-US': 'en-US'
                    }
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(MetricCardGroup);
    });

    it('renders cards only for groups with metrics', () => {
        fixture.componentRef.setInput('tabs', [
            { id: 'content', label: 'Content', cards: [{ id: 'pages', title: 'Pages', data: [{ name: '/', value: 3 }] }] },
            { id: 'network', label: 'Network', cards: [] }
        ]);
        fixture.detectChanges();

        expect(fixture.debugElement.queryAll(By.css('p-card')).length).toBe(1);
        expect(fixture.debugElement.queryAll(By.css('p-tab')).length).toBe(0);
        expect(fixture.nativeElement.textContent).toContain('Content');
        expect(fixture.nativeElement.textContent).toContain('Pages');
        expect(fixture.nativeElement.textContent).not.toContain('Network');
    });

    it('renders related metrics as tabs inside one card', () => {
        fixture.componentRef.setInput('tabs', [
            {
                id: 'location',
                label: 'Location',
                cards: [
                    { id: 'countries', title: 'Countries', data: [{ name: 'DE', value: 2 }] },
                    { id: 'cities', title: 'Cities', data: [{ name: 'Berlin', value: 1 }] }
                ]
            }
        ]);
        fixture.detectChanges();

        expect(fixture.debugElement.queryAll(By.css('p-card')).length).toBe(1);
        expect(fixture.debugElement.queryAll(By.css('p-tab')).length).toBe(2);
        expect(fixture.nativeElement.textContent).toContain('Location');
        expect(fixture.nativeElement.textContent).toContain('Countries');
    });

    it('renders loading and empty metric cards through MetricList', () => {
        fixture.componentRef.setInput('tabs', [
            {
                id: 'audience',
                label: 'Audience',
                cards: [
                    { id: 'devices', title: 'Devices', data: [], isLoading: true },
                    { id: 'browsers', title: 'Browsers', data: [] }
                ]
            }
        ]);
        fixture.detectChanges();

        expect(fixture.debugElement.queryAll(By.css('p-skeleton')).length).toBeGreaterThan(0);

        const browserTab = fixture.debugElement.queryAll(By.css('p-tab')).find((tab) => tab.nativeElement.textContent.includes('Browsers'));
        browserTab?.nativeElement.click();
        fixture.detectChanges();

        expect(fixture.debugElement.query(By.css('.metric-list__row--empty'))).not.toBeNull();
    });

    it('keeps inactive metrics in a tabbed card out of the DOM until selected', () => {
        fixture.componentRef.setInput('tabs', [
            {
                id: 'content',
                label: 'Content',
                cards: [
                    { id: 'pages', title: 'Pages', data: [{ name: '/', value: 3 }] },
                    { id: 'landing', title: 'Landing pages', data: [{ name: '/pricing', value: 2 }] }
                ]
            }
        ]);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Pages');
        expect(fixture.nativeElement.textContent).not.toContain('/pricing');

        const landingTab = fixture.debugElement.queryAll(By.css('p-tab')).find((tab) => tab.nativeElement.textContent.includes('Landing pages'));
        landingTab?.nativeElement.click();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('/pricing');
    });

    it('falls back to the first available metric when the selected metric disappears', () => {
        fixture.componentRef.setInput('tabs', [
            {
                id: 'content',
                label: 'Content',
                cards: [
                    { id: 'pages', title: 'Pages', data: [{ name: '/', value: 3 }] },
                    { id: 'landing', title: 'Landing pages', data: [{ name: '/pricing', value: 2 }] }
                ]
            }
        ]);
        fixture.detectChanges();

        const landingTab = fixture.debugElement.queryAll(By.css('p-tab')).find((tab) => tab.nativeElement.textContent.includes('Landing pages'));
        landingTab?.nativeElement.click();
        fixture.detectChanges();
        expect(fixture.nativeElement.textContent).toContain('/pricing');

        fixture.componentRef.setInput('tabs', [
            {
                id: 'content',
                label: 'Content',
                cards: [{ id: 'pages', title: 'Pages', data: [{ name: '/', value: 3 }] }]
            }
        ]);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Pages');
        expect(fixture.nativeElement.textContent).not.toContain('/pricing');
    });

    it('passes the provenance flag through to single and tabbed metric lists', () => {
        fixture.componentRef.setInput('tabs', [
            {
                id: 'ai-activity',
                label: 'AI activity',
                cards: [
                    { id: 'agents', title: 'AI agents', data: [{ name: 'GPTBot', value: 50, tracked_hits: 10, fetch_count: 40 }], showProvenance: true },
                    { id: 'paths', title: 'Pages crawled', data: [{ name: '/docs', value: 10, tracked_hits: 2, fetch_count: 8 }], showProvenance: true }
                ]
            },
            {
                id: 'plain',
                label: 'Plain',
                cards: [{ id: 'devices', title: 'Devices', data: [{ name: 'Desktop', value: 3, tracked_hits: 3, fetch_count: 1 }] }]
            }
        ]);
        fixture.detectChanges();

        const hints = fixture.debugElement.queryAll(By.css('.metric-list__provenance'));
        expect(hints.length).toBe(1);
        expect(hints[0].nativeElement.textContent.trim()).toBe('tracked 10 · logs 40');

        const pathsTab = fixture.debugElement.queryAll(By.css('p-tab')).find((tab) => tab.nativeElement.textContent.includes('Pages crawled'));
        pathsTab?.nativeElement.click();
        fixture.detectChanges();

        expect(fixture.debugElement.queryAll(By.css('.metric-list__provenance')).map((hint) => hint.nativeElement.textContent.trim())).toContain('tracked 2 · logs 8');
    });

    it('passes the share-column flag through per card and keeps it on by default', () => {
        fixture.componentRef.setInput('tabs', [
            {
                id: 'correlation',
                label: 'Correlation breakdowns',
                cards: [
                    { id: 'citation-yield', title: 'Citation yield', data: [{ name: '/docs', value: 6 }], showShare: false },
                    { id: 'opportunity-pages', title: 'Opportunity pages', data: [{ name: '/pricing', value: 30 }], showShare: false }
                ]
            },
            {
                id: 'plain',
                label: 'Plain',
                cards: [{ id: 'devices', title: 'Devices', data: [{ name: 'Desktop', value: 3 }] }]
            }
        ]);
        fixture.detectChanges();

        // Only the plain group keeps a share column; the correlation cards drop it.
        const shares = fixture.debugElement.queryAll(By.css('.metric-list__share'));
        expect(shares.length).toBe(1);
        expect(shares[0].nativeElement.textContent.trim()).toBe('100%');
    });

    it('emits normalized row click events', () => {
        const emitted: unknown[] = [];
        fixture.componentInstance.rowClicked.subscribe((event) => emitted.push(event));
        fixture.componentRef.setInput('tabs', [
            {
                id: 'location',
                label: 'Location',
                cards: [
                    {
                        id: 'countries',
                        title: 'Countries',
                        data: [{ name: 'DE', value: 2 }],
                        isRowClickable: true,
                        filterType: 'country'
                    }
                ]
            }
        ]);
        fixture.detectChanges();

        fixture.debugElement.query(By.css('.metric-list__row')).nativeElement.click();

        expect(emitted).toEqual([
            {
                tabId: 'location',
                cardId: 'countries',
                filterType: 'country',
                metric: { name: 'DE', value: 2 }
            }
        ]);
    });
});
