package container

import (
	"reflect"

	"github.com/laborin/emed/internal/logging"
	"github.com/laborin/emed/internal/ui"
	"github.com/laborin/emed/internal/ui/components"
	"github.com/laborin/emed/pkg/interfaces"
	"github.com/rivo/tview"
)

func RegisterServices(container *DIContainer) error {
	if err := container.RegisterInstance(
		reflect.TypeOf((*interfaces.Logger)(nil)).Elem(),
		logging.GetLogger(),
	); err != nil {
		return err
	}

	if err := container.RegisterSingleton(
		reflect.TypeOf((*ui.StatusBar)(nil)),
		func(c interfaces.Container) (interface{}, error) {
			return ui.NewStatusBar(), nil
		},
	); err != nil {
		return err
	}

	if err := container.RegisterSingleton(
		reflect.TypeOf((*components.BaseTabManager)(nil)),
		func(c interfaces.Container) (interface{}, error) {
			return components.NewBaseTabManager(), nil
		},
	); err != nil {
		return err
	}

	if err := container.RegisterSingleton(
		reflect.TypeOf((*ui.FileManager)(nil)),
		func(c interfaces.Container) (interface{}, error) {
			return ui.NewFileManager(), nil
		},
	); err != nil {
		return err
	}

	if err := container.RegisterSingleton(
		reflect.TypeOf((*ui.Editor)(nil)),
		func(c interfaces.Container) (interface{}, error) {
			return ui.NewEditor(), nil
		},
	); err != nil {
		return err
	}

	if err := container.RegisterSingleton(
		reflect.TypeOf((*ui.Terminal)(nil)),
		func(c interfaces.Container) (interface{}, error) {
			return ui.NewTerminal(), nil
		},
	); err != nil {
		return err
	}

	if err := container.RegisterSingleton(
		reflect.TypeOf((*ui.Layout)(nil)),
		func(c interfaces.Container) (interface{}, error) {
			app, err := c.Resolve(reflect.TypeOf((*tview.Application)(nil)))
			if err != nil {
				return nil, err
			}
			fileManager, err := c.Resolve(reflect.TypeOf((*ui.FileManager)(nil)))
			if err != nil {
				return nil, err
			}
			editor, err := c.Resolve(reflect.TypeOf((*ui.Editor)(nil)))
			if err != nil {
				return nil, err
			}
			terminal, err := c.Resolve(reflect.TypeOf((*ui.Terminal)(nil)))
			if err != nil {
				return nil, err
			}
			statusBar, err := c.Resolve(reflect.TypeOf((*ui.StatusBar)(nil)))
			if err != nil {
				return nil, err
			}

			return ui.NewLayoutWithDependencies(
				app.(*tview.Application),
				fileManager.(*ui.FileManager),
				editor.(*ui.Editor),
				terminal.(*ui.Terminal),
				statusBar.(*ui.StatusBar),
			), nil
		},
	); err != nil {
		return err
	}

	return nil
}