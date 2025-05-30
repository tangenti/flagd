package store

import (
	"reflect"
	"testing"

	"github.com/open-feature/flagd/core/pkg/logger"
	"github.com/open-feature/flagd/core/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestHasPriority(t *testing.T) {
	tests := []struct {
		name         string
		currentState *State
		storedSource string
		newSource    string
		hasPriority  bool
	}{
		{
			name:         "same source",
			currentState: &State{},
			storedSource: "A",
			newSource:    "A",
			hasPriority:  true,
		},
		{
			name: "no priority",
			currentState: &State{
				FlagSources: []string{
					"B",
					"A",
				},
			},
			storedSource: "A",
			newSource:    "B",
			hasPriority:  false,
		},
		{
			name: "priority",
			currentState: &State{
				FlagSources: []string{
					"A",
					"B",
				},
			},
			storedSource: "A",
			newSource:    "B",
			hasPriority:  true,
		},
		{
			name: "not in sources",
			currentState: &State{
				FlagSources: []string{
					"A",
					"B",
				},
			},
			storedSource: "C",
			newSource:    "D",
			hasPriority:  true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := tt.currentState.hasPriority(tt.storedSource, tt.newSource)
			require.Equal(t, p, tt.hasPriority)
		})
	}
}

func TestMergeFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		current     *State
		new         map[string]model.Flag
		newSource   string
		newSelector string
		want        *State
		wantNotifs  map[string]interface{}
		wantResync  bool
	}{
		{
			name:       "both nil",
			current:    &State{Flags: nil},
			new:        nil,
			want:       &State{Flags: nil},
			wantNotifs: map[string]interface{}{},
		},
		{
			name:       "both empty flags",
			current:    &State{Flags: map[string]model.Flag{}},
			new:        map[string]model.Flag{},
			want:       &State{Flags: map[string]model.Flag{}},
			wantNotifs: map[string]interface{}{},
		},
		{
			name:       "empty new",
			current:    &State{Flags: map[string]model.Flag{}},
			new:        nil,
			want:       &State{Flags: map[string]model.Flag{}},
			wantNotifs: map[string]interface{}{},
		},
		{
			name: "merging with new source",
			current: &State{
				Flags: map[string]model.Flag{
					"waka": {
						DefaultVariant: "off",
						Source:         "1",
					},
				},
			},
			new: map[string]model.Flag{
				"paka": {
					DefaultVariant: "on",
				},
			},
			newSource: "2",
			want: &State{Flags: map[string]model.Flag{
				"waka": {
					DefaultVariant: "off",
					Source:         "1",
				},
				"paka": {
					DefaultVariant: "on",
					Source:         "2",
				},
			}},
			wantNotifs: map[string]interface{}{"paka": map[string]interface{}{"type": "write", "source": "2"}},
		},
		{
			name: "override by new update",
			current: &State{Flags: map[string]model.Flag{
				"waka": {DefaultVariant: "off"},
				"paka": {DefaultVariant: "off"},
			}},
			new: map[string]model.Flag{
				"waka": {DefaultVariant: "on"},
				"paka": {DefaultVariant: "on"},
			},
			want: &State{Flags: map[string]model.Flag{
				"waka": {DefaultVariant: "on"},
				"paka": {DefaultVariant: "on"},
			}},
			wantNotifs: map[string]interface{}{
				"waka": map[string]interface{}{"type": "update", "source": ""},
				"paka": map[string]interface{}{"type": "update", "source": ""},
			},
		},
		{
			name: "identical update so empty notifications",
			current: &State{
				Flags: map[string]model.Flag{"hello": {DefaultVariant: "off"}},
			},
			new: map[string]model.Flag{
				"hello": {DefaultVariant: "off"},
			},
			want: &State{Flags: map[string]model.Flag{
				"hello": {DefaultVariant: "off"},
			}},
			wantNotifs: map[string]interface{}{},
		},
		{
			name:       "deleted flag & trigger resync for same source",
			current:    &State{Flags: map[string]model.Flag{"hello": {DefaultVariant: "off", Source: "A"}}},
			new:        map[string]model.Flag{},
			newSource:  "A",
			want:       &State{Flags: map[string]model.Flag{}},
			wantNotifs: map[string]interface{}{"hello": map[string]interface{}{"type": "delete", "source": "A"}},
			wantResync: true,
		},
		{
			name:        "no deleted & no resync for same source but different selector",
			current:     &State{Flags: map[string]model.Flag{"hello": {DefaultVariant: "off", Source: "A", Selector: "X"}}},
			new:         map[string]model.Flag{},
			newSource:   "A",
			newSelector: "Y",
			want:        &State{Flags: map[string]model.Flag{"hello": {DefaultVariant: "off", Source: "A", Selector: "X"}}},
			wantResync:  false,
			wantNotifs:  map[string]interface{}{},
		},
		{
			name: "no merge due to low priority",
			current: &State{
				FlagSources: []string{
					"B",
					"A",
				},
				Flags: map[string]model.Flag{
					"hello": {
						DefaultVariant: "off",
						Source:         "A",
					},
				},
			},
			new:       map[string]model.Flag{"hello": {DefaultVariant: "off"}},
			newSource: "B",
			want: &State{
				FlagSources: []string{
					"B",
					"A",
				},
				Flags: map[string]model.Flag{
					"hello": {
						DefaultVariant: "off",
						Source:         "A",
					},
				},
			},
			wantNotifs: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotNotifs, resyncRequired := tt.current.Merge(logger.NewLogger(nil, false), tt.newSource, tt.newSelector, tt.new, model.Metadata{})

			require.True(t, reflect.DeepEqual(tt.want.Flags, tt.current.Flags))
			require.Equal(t, tt.wantNotifs, gotNotifs)
			require.Equal(t, tt.wantResync, resyncRequired)
		})
	}
}

func TestFlags_Add(t *testing.T) {
	mockLogger := logger.NewLogger(nil, false)
	mockSource := "source"
	mockOverrideSource := "source-2"

	type request struct {
		source   string
		selector string
		flags    map[string]model.Flag
	}

	tests := []struct {
		name                     string
		storedState              *State
		addRequest               request
		expectedState            *State
		expectedNotificationKeys []string
	}{
		{
			name: "Add success",
			storedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource},
				},
			},
			addRequest: request{
				source: mockSource,
				flags: map[string]model.Flag{
					"B": {Source: mockSource},
				},
			},
			expectedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource},
					"B": {Source: mockSource},
				},
			},
			expectedNotificationKeys: []string{"B"},
		},
		{
			name: "Add multiple success",
			storedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource},
				},
			},
			addRequest: request{
				source: mockSource,
				flags: map[string]model.Flag{
					"B": {Source: mockSource},
					"C": {Source: mockSource},
				},
			},
			expectedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource},
					"B": {Source: mockSource},
					"C": {Source: mockSource},
				},
			},
			expectedNotificationKeys: []string{"B", "C"},
		},
		{
			name: "Add success - conflict and override",
			storedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource},
				},
			},
			addRequest: request{
				source: mockOverrideSource,
				flags: map[string]model.Flag{
					"A": {Source: mockOverrideSource},
				},
			},
			expectedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockOverrideSource},
				},
			},
			expectedNotificationKeys: []string{"A"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := tt.storedState.Add(mockLogger, tt.addRequest.source, tt.addRequest.selector, tt.addRequest.flags)

			require.Equal(t, tt.storedState, tt.expectedState)

			for k := range messages {
				require.Containsf(t, tt.expectedNotificationKeys, k,
					"Message key %s not present in the expected key list", k)
			}
		})
	}
}

func TestFlags_Update(t *testing.T) {
	mockLogger := logger.NewLogger(nil, false)
	mockSource := "source"
	mockOverrideSource := "source-2"

	type request struct {
		source   string
		selector string
		flags    map[string]model.Flag
	}

	tests := []struct {
		name                     string
		storedState              *State
		UpdateRequest            request
		expectedState            *State
		expectedNotificationKeys []string
	}{
		{
			name: "Update success",
			storedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource, DefaultVariant: "True"},
				},
			},
			UpdateRequest: request{
				source: mockSource,
				flags: map[string]model.Flag{
					"A": {Source: mockSource, DefaultVariant: "False"},
				},
			},
			expectedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource, DefaultVariant: "False"},
				},
			},
			expectedNotificationKeys: []string{"A"},
		},
		{
			name: "Update multiple success",
			storedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource, DefaultVariant: "True"},
					"B": {Source: mockSource, DefaultVariant: "True"},
				},
			},
			UpdateRequest: request{
				source: mockSource,
				flags: map[string]model.Flag{
					"A": {Source: mockSource, DefaultVariant: "False"},
					"B": {Source: mockSource, DefaultVariant: "False"},
				},
			},
			expectedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource, DefaultVariant: "False"},
					"B": {Source: mockSource, DefaultVariant: "False"},
				},
			},
			expectedNotificationKeys: []string{"A", "B"},
		},
		{
			name: "Update success - conflict and override",
			storedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource, DefaultVariant: "True"},
				},
			},
			UpdateRequest: request{
				source: mockOverrideSource,
				flags: map[string]model.Flag{
					"A": {Source: mockOverrideSource, DefaultVariant: "True"},
				},
			},
			expectedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockOverrideSource, DefaultVariant: "True"},
				},
			},
			expectedNotificationKeys: []string{"A"},
		},
		{
			name: "Update fail",
			storedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource},
				},
			},
			UpdateRequest: request{
				source: mockSource,
				flags: map[string]model.Flag{
					"B": {Source: mockSource},
				},
			},
			expectedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource},
				},
			},
			expectedNotificationKeys: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := tt.storedState.Update(mockLogger, tt.UpdateRequest.source,
				tt.UpdateRequest.selector, tt.UpdateRequest.flags)

			require.Equal(t, tt.storedState, tt.expectedState)

			for k := range messages {
				require.Containsf(t, tt.expectedNotificationKeys, k,
					"Message key %s not present in the expected key list", k)
			}
		})
	}
}

func TestFlags_Delete(t *testing.T) {
	mockLogger := logger.NewLogger(nil, false)
	mockSource := "source"
	mockSource2 := "source2"

	tests := []struct {
		name                     string
		storedState              *State
		deleteRequest            map[string]model.Flag
		expectedState            *State
		expectedNotificationKeys []string
	}{
		{
			name: "Remove success",
			storedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource},
					"B": {Source: mockSource},
					"C": {Source: mockSource2},
				},
				FlagSources: []string{
					mockSource,
					mockSource2,
				},
			},
			deleteRequest: map[string]model.Flag{
				"A": {Source: mockSource},
			},
			expectedState: &State{
				Flags: map[string]model.Flag{
					"B": {Source: mockSource},
					"C": {Source: mockSource2},
				},
				FlagSources: []string{
					mockSource,
					mockSource2,
				},
			},
			expectedNotificationKeys: []string{"A"},
		},
		{
			name: "Nothing to remove",
			storedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource},
					"B": {Source: mockSource},
					"C": {Source: mockSource2},
				},
				FlagSources: []string{
					mockSource,
					mockSource2,
				},
			},
			deleteRequest: map[string]model.Flag{
				"C": {Source: mockSource},
			},
			expectedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource},
					"B": {Source: mockSource},
					"C": {Source: mockSource2},
				},
				FlagSources: []string{
					mockSource,
					mockSource2,
				},
			},
			expectedNotificationKeys: []string{},
		},
		{
			name: "Remove all",
			storedState: &State{
				Flags: map[string]model.Flag{
					"A": {Source: mockSource},
					"B": {Source: mockSource},
					"C": {Source: mockSource2},
				},
			},
			deleteRequest: map[string]model.Flag{},
			expectedState: &State{
				Flags: map[string]model.Flag{
					"C": {Source: mockSource2},
				},
			},
			expectedNotificationKeys: []string{"A", "B"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := tt.storedState.DeleteFlags(mockLogger, mockSource, tt.deleteRequest)

			require.Equal(t, tt.storedState, tt.expectedState)

			for k := range messages {
				require.Containsf(t, tt.expectedNotificationKeys, k,
					"Message key %s not present in the expected key list", k)
			}
		})
	}
}
