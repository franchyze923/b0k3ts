import { ComponentFixture, TestBed } from '@angular/core/testing';

import { MovePrefixDialog } from './move-prefix-dialog';

describe('MovePrefixDialog', () => {
  let component: MovePrefixDialog;
  let fixture: ComponentFixture<MovePrefixDialog>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [MovePrefixDialog]
    })
    .compileComponents();

    fixture = TestBed.createComponent(MovePrefixDialog);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
