import { ComponentFixture, TestBed } from '@angular/core/testing';

import { OidcSettings } from './oidc-settings';

describe('OidcSettings', () => {
  let component: OidcSettings;
  let fixture: ComponentFixture<OidcSettings>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [OidcSettings]
    })
    .compileComponents();

    fixture = TestBed.createComponent(OidcSettings);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
