# gomobile-generated bindings are called from native code by name.
-keep class go.** { *; }
-keep class admmobile.** { *; }
# Methods exposed to the WebView.
-keepclassmembers class io.github.adm.MainActivity$Bridge {
    @android.webkit.JavascriptInterface <methods>;
}
