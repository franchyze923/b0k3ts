import { ComponentFixture, TestBed } from '@angular/core/testing';

import { Cephcredssnack } from './cephcredssnack';

describe('Cephcredssnack', () => {
  let component: Cephcredssnack;
  let fixture: ComponentFixture<Cephcredssnack>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [Cephcredssnack]
    })
    .compileComponents();

    fixture = TestBed.createComponent(Cephcredssnack);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
