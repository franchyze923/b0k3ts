import { ComponentFixture, TestBed } from '@angular/core/testing';

import { Ceph } from './ceph';

describe('Ceph', () => {
  let component: Ceph;
  let fixture: ComponentFixture<Ceph>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [Ceph]
    })
    .compileComponents();

    fixture = TestBed.createComponent(Ceph);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
