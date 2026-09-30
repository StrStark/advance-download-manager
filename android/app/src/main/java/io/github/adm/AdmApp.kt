package io.github.adm

import android.app.Application

class AdmApp : Application() {
    override fun onCreate() {
        super.onCreate()
        Notifications.createChannels(this)
    }
}
