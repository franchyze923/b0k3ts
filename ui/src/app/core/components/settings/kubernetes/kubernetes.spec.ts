import { ComponentFixture, TestBed } from '@angular/core/testing';

import { Kubernetes } from './kubernetes';

describe('Kubernetes', () => {
  let component: Kubernetes;
  let fixture: ComponentFixture<Kubernetes>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [Kubernetes]
    })
    .compileComponents();

    fixture = TestBed.createComponent(Kubernetes);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
