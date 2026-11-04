# The planted regression: the team removes the customer index, believing the composite index covers it.
from django.db import migrations


class Migration(migrations.Migration):
    dependencies = [
        ("shop", "0001_initial"),
    ]

    operations = [
        migrations.RemoveIndex(
            model_name="order",
            name="orders_customer_idx",
        ),
    ]
