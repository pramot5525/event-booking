package service

import (
"errors"
"event-service/internal/model"
repomocks "event-service/internal/repository/mocks"
"testing"

"github.com/stretchr/testify/require"
"github.com/stretchr/testify/suite"
)

type EventServiceSuite struct {
suite.Suite
}

func TestEventServiceSuite(t *testing.T) {
suite.Run(t, new(EventServiceSuite))
}

func (s *EventServiceSuite) TestGetEvents_TableDriven() {
expected := []*model.Event{{ID: 2, Name: "B"}, {ID: 1, Name: "A"}}

tests := []struct {
name    string
setup   func(repo *repomocks.EventRepository)
want    []*model.Event
wantErr bool
}{
{
name: "success",
setup: func(repo *repomocks.EventRepository) {
repo.EXPECT().GetAll().Return(expected, nil)
},
want: expected,
},
{
name: "repository error",
setup: func(repo *repomocks.EventRepository) {
repo.EXPECT().GetAll().Return(nil, errors.New("db error"))
},
wantErr: true,
},
}

for _, tc := range tests {
s.T().Run(tc.name, func(t *testing.T) {
repo := repomocks.NewEventRepository(t)
tc.setup(repo)
svc := NewEventService(repo)

got, err := svc.GetEvents()
if tc.wantErr {
require.Error(t, err)
return
}

require.NoError(t, err)
require.Equal(t, tc.want, got)
})
}
}

func (s *EventServiceSuite) TestGetEvent_TableDriven() {
expected := &model.Event{ID: 10, Name: "Go Conf"}

tests := []struct {
name    string
id      int64
setup   func(repo *repomocks.EventRepository)
want    *model.Event
wantErr bool
}{
{
name: "success",
id:   10,
setup: func(repo *repomocks.EventRepository) {
repo.EXPECT().GetByID(int64(10)).Return(expected, nil)
},
want: expected,
},
{
name: "repository error",
id:   10,
setup: func(repo *repomocks.EventRepository) {
repo.EXPECT().GetByID(int64(10)).Return(nil, errors.New("not found"))
},
wantErr: true,
},
}

for _, tc := range tests {
s.T().Run(tc.name, func(t *testing.T) {
repo := repomocks.NewEventRepository(t)
tc.setup(repo)
svc := NewEventService(repo)

got, err := svc.GetEvent(tc.id)
if tc.wantErr {
require.Error(t, err)
return
}

require.NoError(t, err)
require.Equal(t, tc.want, got)
})
}
}

func (s *EventServiceSuite) TestCreateEvent_TableDriven() {
tests := []struct {
name    string
input   *model.Event
setup   func(repo *repomocks.EventRepository, input *model.Event)
wantID  *int64
wantErr bool
}{
{
name:  "success",
input: &model.Event{Name: "New Event", SeatLimit: 100},
setup: func(repo *repomocks.EventRepository, input *model.Event) {
id := int64(77)
repo.EXPECT().Create(input).Return(&id, nil)
},
wantID: func() *int64 { v := int64(77); return &v }(),
},
{
name:  "repository error",
input: &model.Event{Name: "Broken"},
setup: func(repo *repomocks.EventRepository, input *model.Event) {
repo.EXPECT().Create(input).Return(nil, errors.New("insert failed"))
},
wantErr: true,
},
}

for _, tc := range tests {
s.T().Run(tc.name, func(t *testing.T) {
repo := repomocks.NewEventRepository(t)
tc.setup(repo, tc.input)
svc := NewEventService(repo)

gotID, err := svc.CreateEvent(tc.input)
if tc.wantErr {
require.Error(t, err)
require.Nil(t, gotID)
return
}

require.NoError(t, err)
require.NotNil(t, gotID)
require.Equal(t, *tc.wantID, *gotID)
})
}
}

func (s *EventServiceSuite) TestUpdateEvent_TableDriven() {
tests := []struct {
name    string
input   *model.Event
setup   func(repo *repomocks.EventRepository, input *model.Event)
wantErr bool
}{
{
name:  "success",
input: &model.Event{ID: 3, Name: "Updated"},
setup: func(repo *repomocks.EventRepository, input *model.Event) {
repo.EXPECT().Update(input).Return(nil)
},
},
{
name:  "repository error",
input: &model.Event{ID: 3, Name: "Updated"},
setup: func(repo *repomocks.EventRepository, input *model.Event) {
repo.EXPECT().Update(input).Return(errors.New("update failed"))
},
wantErr: true,
},
}

for _, tc := range tests {
s.T().Run(tc.name, func(t *testing.T) {
repo := repomocks.NewEventRepository(t)
tc.setup(repo, tc.input)
svc := NewEventService(repo)

err := svc.UpdateEvent(tc.input)
if tc.wantErr {
require.Error(t, err)
return
}

require.NoError(t, err)
})
}
}

func (s *EventServiceSuite) TestDeleteEvent_TableDriven() {
tests := []struct {
name    string
id      int64
setup   func(repo *repomocks.EventRepository, id int64)
wantErr bool
}{
{
name: "success",
id:   5,
setup: func(repo *repomocks.EventRepository, id int64) {
repo.EXPECT().Delete(id).Return(nil)
},
},
{
name: "repository error",
id:   5,
setup: func(repo *repomocks.EventRepository, id int64) {
repo.EXPECT().Delete(id).Return(errors.New("delete failed"))
},
wantErr: true,
},
}

for _, tc := range tests {
s.T().Run(tc.name, func(t *testing.T) {
repo := repomocks.NewEventRepository(t)
tc.setup(repo, tc.id)
svc := NewEventService(repo)

err := svc.DeleteEvent(tc.id)
if tc.wantErr {
require.Error(t, err)
return
}

require.NoError(t, err)
})
}
}
