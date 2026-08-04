// Package scriptparams parses parameter metadata embedded in Frida JavaScript
// or TypeScript comments and safely renders parameter placeholders.
//
// Metadata is a JSON object following an @frida-gui marker in either a block
// comment or a consecutive run of line comments:
//
//	/*
//	@frida-gui
//	{
//	  "name": "Module inspector",
//	  "parameters": [
//	    {
//	      "name": "packageName",
//	      "label": "Package name",
//	      "placeholder": "com.example.app",
//	      "type": "string",
//	      "required": true
//	    },
//	    {
//	      "name": "moduleName",
//	      "label": "Module",
//	      "type": "select",
//	      "default": "libtarget.so",
//	      "options": ["libtarget.so", "libother.so"]
//	    }
//	  ]
//	}
//	*/
//	const packageName = {{packageName}};
//	const moduleName = {{moduleName}};
//
// Placeholders must not be wrapped in quotes. Render replaces each placeholder
// with a JavaScript literal, including the required escaping for string values.
// Supported parameter types are string, number, boolean, and select. Select
// values are strings and must be one of the declared options.
package scriptparams
