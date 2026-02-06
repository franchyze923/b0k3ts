import { ComponentFixture, TestBed } from '@angular/core/testing';

import { BucketConnectionsSettings } from './bucket-connections-settings';

describe('BucketConnectionsSettings', () => {
  let component: BucketConnectionsSettings;
  let fixture: ComponentFixture<BucketConnectionsSettings>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [BucketConnectionsSettings]
    })
    .compileComponents();

    fixture = TestBed.createComponent(BucketConnectionsSettings);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
