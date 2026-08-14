import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:shellsync_mobile/app.dart';

void main() {
  testWidgets('app boots to loading screen', (WidgetTester tester) async {
    await tester.pumpWidget(const ShellSyncApp());
    // AppState 尚未初始化时应显示加载指示器，而不是抛异常
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
  });
}
