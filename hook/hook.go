package hook

import "fmt"

func Call(hook Interface) (err error) {
	if hook == nil {
		return fmt.Errorf("hook cannot be nil")
	}

	defer hook.Finally()

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic occurred during hook execution: %v", r)
		}
	}()

	tryErr := hook.Try()
	if tryErr != nil {
		err = hook.Catch(tryErr)
		return err
	}

	return nil
}
